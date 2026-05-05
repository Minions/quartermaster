// Wails v2 frontend — install-minions installer UI.
//
// Go methods are accessed via window.go.main.App.*
// Events from Go are received via window.runtime.EventsOn(...)

'use strict';

// ── State ──────────────────────────────────────────────────────────────────

let dashboardURL = '';
let pendingInstallPath = '';
let activeProvider = 'github';
let oauthVerificationURL = '';

// ── DOM helpers ────────────────────────────────────────────────────────────

const $ = (id) => document.getElementById(id);

function showScreen(name) {
  document.querySelectorAll('.screen').forEach((el) => el.classList.remove('active'));
  $(`screen-${name}`).classList.add('active');
}

function setStepState(stepNum, status, message) {
  const el = document.querySelector(`.step[data-step="${stepNum}"]`);
  if (!el) return;
  el.className = `step ${status}`;
  if (message !== undefined && message !== '') {
    el.querySelector('.step-msg').textContent = message;
  } else if (status === 'running') {
    el.querySelector('.step-msg').textContent = '';
  }
}

function showError(msg) {
  $('error-panel').classList.remove('hidden');
  $('error-text').textContent = msg;
  $('progress-title').textContent = 'Assembly failed';
}

function resetSteps() {
  document.querySelectorAll('.step').forEach((el) => {
    el.className = 'step pending';
    el.querySelector('.step-msg').textContent = '';
  });
  $('error-panel').classList.add('hidden');
  $('error-text').textContent = '';
  $('progress-title').textContent = 'Building your secret Lairs…';
}

// ── Auth screen ────────────────────────────────────────────────────────────

const providerLabels = {
  github:    'GitHub',
  bitbucket: 'Bitbucket',
  gitlab:    'GitLab',
};

const providerDescs = {
  github:    'Sign in with GitHub to grant repository access.',
  bitbucket: 'Sign in with Bitbucket to grant repository access.',
  gitlab:    'Sign in with GitLab to grant repository access.',
};

// OAuth states: idle | device | browser-waiting | error
function setOAuthState(state, payload) {
  $('oauth-idle').classList.add('hidden');
  $('oauth-device').classList.add('hidden');
  $('oauth-browser-waiting').classList.add('hidden');
  $('oauth-error-panel').classList.add('hidden');

  switch (state) {
    case 'idle':
      $('oauth-idle').classList.remove('hidden');
      break;

    case 'device':
      $('oauth-user-code').textContent = payload.userCode;
      oauthVerificationURL = payload.verificationURL;
      $('oauth-device').classList.remove('hidden');
      break;

    case 'browser-waiting':
      $('oauth-browser-waiting').classList.remove('hidden');
      break;

    case 'error':
      $('oauth-error-text').textContent = payload;
      $('oauth-error-panel').classList.remove('hidden');
      break;
  }
}

function selectProvider(provider) {
  activeProvider = provider;

  document.querySelectorAll('.provider-tab').forEach((btn) => {
    btn.classList.toggle('active', btn.dataset.provider === provider);
  });

  if (provider === 'other') {
    $('oauth-panel').classList.add('hidden');
    $('pat-panel').classList.remove('hidden');
  } else {
    $('oauth-panel').classList.remove('hidden');
    $('pat-panel').classList.add('hidden');
    const label = providerLabels[provider] || provider;
    $('oauth-provider-desc').textContent = providerDescs[provider] || `Sign in with ${label}.`;
    $('oauth-signin-btn').textContent = `Sign in with ${label}`;
    setOAuthState('idle');
    // Clear any PAT error
    $('pat-error').classList.add('hidden');
  }
}

function beginInstall() {
  resetSteps();
  showScreen('progress');
  window.go.main.App.StartInstallation(pendingInstallPath);
}

// ── Startup ────────────────────────────────────────────────────────────────

window.addEventListener('DOMContentLoaded', async () => {
  // Pre-fill install path.
  const existing = await window.go.main.App.GetExistingInstallPath();
  const defaultPath = existing || await window.go.main.App.GetDefaultInstallPath();
  $('install-path').value = defaultPath;

  // ── Setup screen ──

  $('browse-btn').addEventListener('click', async () => {
    const current = $('install-path').value.trim() || defaultPath;
    const chosen = await window.go.main.App.ChooseDirectory(current);
    if (chosen) $('install-path').value = chosen;
  });

  $('install-btn').addEventListener('click', () => {
    const path = $('install-path').value.trim();
    if (!path) return;
    pendingInstallPath = path;
    showScreen('auth');
  });

  // ── Auth screen — provider tabs ──

  document.querySelectorAll('.provider-tab').forEach((btn) => {
    btn.addEventListener('click', () => selectProvider(btn.dataset.provider));
  });

  // ── Auth screen — OAuth sign-in ──

  $('oauth-signin-btn').addEventListener('click', () => {
    setOAuthState('idle'); // reset error if retrying
    window.go.main.App.StartOAuth(activeProvider);
  });

  $('oauth-open-browser-btn').addEventListener('click', () => {
    if (oauthVerificationURL) {
      window.go.main.App.OpenURL(oauthVerificationURL);
    }
  });

  $('oauth-retry-btn').addEventListener('click', () => {
    setOAuthState('idle');
  });

  // ── Auth screen — PAT form (Other provider) ──

  $('pat-connect-btn').addEventListener('click', async () => {
    const host     = $('git-host').value.trim();
    const username = $('git-username').value.trim();
    const token    = $('git-token').value.trim();
    const err = await window.go.main.App.SaveGitCredential(host, username, token);
    if (err) {
      $('pat-error').textContent = err;
      $('pat-error').classList.remove('hidden');
      return;
    }
    beginInstall();
  });

  // ── Auth screen — skip ──

  $('skip-auth-btn').addEventListener('click', () => {
    beginInstall();
  });

  // ── Progress screen — retry ──

  $('retry-btn').addEventListener('click', () => {
    showScreen('setup');
  });

  // ── Done screen ──

  $('open-btn').addEventListener('click', () => {
    if (dashboardURL) window.go.main.App.OpenURL(dashboardURL);
  });

  $('close-btn').addEventListener('click', () => {
    window.go.main.App.Quit();
  });

  // ── Events from Go ──

  window.runtime.EventsOn('step', (event) => {
    setStepState(event.step, event.status, event.message);
  });

  window.runtime.EventsOn('done', (url) => {
    dashboardURL = url;
    window.go.main.App.OpenURL(url);
    showScreen('done');
  });

  window.runtime.EventsOn('error', (msg) => {
    showError(msg);
  });

  // OAuth events
  window.runtime.EventsOn('oauth:device_code', (payload) => {
    // payload: { userCode, verificationURL }
    setOAuthState('device', payload);
  });

  window.runtime.EventsOn('oauth:waiting', () => {
    setOAuthState('browser-waiting');
  });

  window.runtime.EventsOn('oauth:complete', () => {
    // Credentials saved — proceed straight to installation.
    beginInstall();
  });

  window.runtime.EventsOn('oauth:error', (msg) => {
    setOAuthState('error', msg);
  });
});
