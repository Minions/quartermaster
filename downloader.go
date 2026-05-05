package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// dominionBundleSHA256 is the expected SHA-256 of dominion-bundle.zip,
// embedded at build time via -ldflags "-X main.dominionBundleSHA256=<hash>".
// If empty (dev builds), integrity checking is skipped.
var dominionBundleSHA256 string

// dominionBundleURL is the versioned install.minions.tools URL for the bundle,
// embedded at build time via -ldflags "-X main.dominionBundleURL=<url>".
// Cloudflare redirects it to the matching GitHub Releases asset; Go's HTTP
// client follows the redirect transparently.
// Falls back to a hard-coded default for dev builds.
var dominionBundleURL = "https://install.minions.tools/v0.1.0/dominion-bundle.zip"

// downloadDominion downloads and extracts the Dominion runtime bundle to destDir.
// If destDir already contains a valid installation, this is a no-op.
// The log callback receives human-readable progress messages.
func downloadDominion(destDir string, log func(string)) error {
	mainJS := filepath.Join(destDir, "dist", "main.js")
	if _, err := os.Stat(mainJS); err == nil {
		log("Dominion already installed — skipping download.")
		return nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dominion dir: %w", err)
	}

	tmpZip := filepath.Join(os.TempDir(), "dominion-bundle.zip")
	log(fmt.Sprintf("Downloading from %s…", dominionBundleURL))

	if err := downloadFile(dominionBundleURL, tmpZip); err != nil {
		return fmt.Errorf("download bundle: %w", err)
	}
	defer os.Remove(tmpZip)

	if dominionBundleSHA256 != "" {
		log("Verifying bundle integrity…")
		if err := verifySHA256(tmpZip, dominionBundleSHA256); err != nil {
			return fmt.Errorf("bundle integrity check failed: %w", err)
		}
		log("Bundle integrity verified.")
	}

	log("Extracting bundle…")
	if err := extractZip(tmpZip, destDir); err != nil {
		return fmt.Errorf("extract bundle: %w", err)
	}

	log("Dominion installed.")
	return nil
}

// verifySHA256 checks that the SHA-256 of the file at path matches expectedHex.
// The comparison is case-insensitive.
func verifySHA256(path, expectedHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != strings.ToLower(expectedHex) {
		return fmt.Errorf("SHA256 mismatch:\n  got      %s\n  expected %s", actual, strings.ToLower(expectedHex))
	}
	return nil
}

// downloadFile downloads a URL to a local file.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s for %s", resp.Status, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// extractZip extracts a zip archive to destDir.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Sanitize path to prevent zip slip
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		if err := extractFile(f, target); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

// extractZipFlat extracts all regular files from a zip archive directly into
// destDir, flattening any directory structure inside the archive. Used to
// unpack standalone binary bundles (e.g. Volta for Windows) without running
// an MSI or requiring admin privileges.
func extractZipFlat(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		target := filepath.Join(destDir, filepath.Base(f.Name))
		if err := extractFile(f, target); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

func extractFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
