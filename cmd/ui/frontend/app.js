// Initialize Lucide Icons
document.addEventListener('DOMContentLoaded', () => {
  if (window.lucide) {
    lucide.createIcons();
  }
});

// App State
const state = {
  metricsHistory: [],
  activityHistory: [],
  processes: [],
  ws: null,
  charts: {
    perf: null,
    appPie: null,
    websitePie: null
  }
};

// DOM Element Selectors
const DOM = {
  statusIndicator: document.getElementById('statusIndicator'),
  statusText: document.getElementById('statusText'),
  agentUptime: document.getElementById('agentUptime'),
  agentSamples: document.getElementById('agentSamples'),
  btnStartAgent: document.getElementById('btnStartAgent'),
  btnStopAgent: document.getElementById('btnStopAgent'),
  btnExportData: document.getElementById('btnExportData'),
  
  // Hero Active App
  currentAppTitle: document.getElementById('currentAppTitle'),
  currentProcessName: document.getElementById('currentProcessName'),
  currentPID: document.getElementById('currentPID'),
  userIdleState: document.getElementById('userIdleState'),
  
  // Metric Cards
  valCpu: document.getElementById('valCpu'),
  barCpu: document.getElementById('barCpu'),
  valRam: document.getElementById('valRam'),
  valRamSub: document.getElementById('valRamSub'),
  barRam: document.getElementById('barRam'),
  valDisk: document.getElementById('valDisk'),
  valDiskSub: document.getElementById('valDiskSub'),
  barDisk: document.getElementById('barDisk'),
  valPower: document.getElementById('valPower'),
  valBatterySub: document.getElementById('valBatterySub'),
  
  // Tables
  appActivityTableBody: document.getElementById('appActivityTableBody'),
  processTableBody: document.getElementById('processTableBody'),
  metricsTableBody: document.getElementById('metricsTableBody')
};

// Navigation Tab Switching
document.querySelectorAll('.nav-item').forEach(item => {
  item.addEventListener('click', (e) => {
    e.preventDefault();
    document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
    document.querySelectorAll('.tab-section').forEach(s => s.classList.remove('active'));
    
    item.classList.add('active');
    const tabName = item.getAttribute('data-tab');
    const targetSection = document.getElementById(`tab-${tabName}`);
    if (targetSection) targetSection.classList.add('active');
  });
});

// Initialize Chart.js Graphs
function initCharts() {
  const ctxPerfEl = document.getElementById('perfChart');
  if (ctxPerfEl) {
    const ctxPerf = ctxPerfEl.getContext('2d');
    state.charts.perf = new Chart(ctxPerf, {
      type: 'line',
      data: {
        labels: [],
        datasets: [
          {
            label: 'CPU Usage (%)',
            borderColor: '#00f0ff',
            backgroundColor: 'rgba(0, 240, 255, 0.1)',
            borderWidth: 2,
            fill: true,
            tension: 0.4,
            data: []
          },
          {
            label: 'RAM Usage (%)',
            borderColor: '#a855f7',
            backgroundColor: 'rgba(168, 85, 247, 0.1)',
            borderWidth: 2,
            fill: true,
            tension: 0.4,
            data: []
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: {
            grid: { color: 'rgba(255, 255, 255, 0.05)' },
            ticks: { color: '#64748b', maxTicksLimit: 10 }
          },
          y: {
            min: 0,
            max: 100,
            grid: { color: 'rgba(255, 255, 255, 0.05)' },
            ticks: { color: '#64748b' }
          }
        },
        plugins: {
          legend: { labels: { color: '#f1f5f9' } }
        }
      }
    });
  }

  const ctxPieEl = document.getElementById('appPieChart');
  if (ctxPieEl) {
    const ctxPie = ctxPieEl.getContext('2d');
    state.charts.appPie = new Chart(ctxPie, {
      type: 'doughnut',
      data: {
        labels: ['Detecting...'],
        datasets: [{
          data: [100],
          backgroundColor: ['#00f0ff', '#a855f7', '#10b981', '#f59e0b', '#ec4899', '#3b82f6'],
          borderWidth: 0
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { position: 'right', labels: { color: '#f1f5f9' } }
        }
      }
    });
  }

  const ctxWebPieEl = document.getElementById('websitePieChart');
  if (ctxWebPieEl) {
    const ctxWebPie = ctxWebPieEl.getContext('2d');
    state.charts.websitePie = new Chart(ctxWebPie, {
      type: 'doughnut',
      data: {
        labels: ['Detecting...'],
        datasets: [{
          data: [100],
          backgroundColor: ['#00f0ff', '#a855f7', '#10b981', '#f59e0b', '#ec4899', '#3b82f6', '#8b5cf6'],
          borderWidth: 0
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { position: 'right', labels: { color: '#f1f5f9' } }
        }
      }
    });
  }
}

// Connect to Real-time WebSocket Stream with HTTP Polling Fallback
function connectWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;
  
  try {
    state.ws = new WebSocket(wsUrl);

    state.ws.onopen = () => {
      console.log('[WS] Connected to Go-Agent real-time telemetry stream.');
    };

    state.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'initial_state') {
          handleInitialState(msg);
        } else if (msg.type === 'telemetry') {
          handleTelemetryFrame(msg);
        } else if (msg.type === 'process_snapshot') {
          handleProcessSnapshot(msg.data);
        }
      } catch (err) {
        console.error('[WS] Error parsing frame:', err);
      }
    };

    state.ws.onclose = () => {
      console.warn('[WS] Stream disconnected. Reconnecting in 3s...');
      setTimeout(connectWebSocket, 3000);
    };

    state.ws.onerror = (err) => {
      console.warn('[WS] WebSocket error, using HTTP polling fallback.');
    };
  } catch (err) {
    console.warn('[WS] Unable to init WebSocket, using HTTP polling fallback.');
  }

  // HTTP Polling fallback loop every 1.5s
  setInterval(fetchTelemetryPoll, 1500);
}

async function fetchTelemetryPoll() {
  if (state.ws && state.ws.readyState === WebSocket.OPEN) return;
  try {
    const res = await fetch('/api/telemetry');
    const msg = await res.json();
    if (msg) {
      if (msg.agentStatus) updateAgentStatus(msg.agentStatus);
      if (msg.data) handleTelemetryFrame({ agentStatus: msg.agentStatus, data: msg.data });
      if (msg.processes) renderProcessTable(msg.processes);
    }
  } catch (err) {
    console.error('[Poll] Telemetry poll error:', err);
  }
}

// Handle Initial State Sync
function handleInitialState(msg) {
  updateAgentStatus(msg.agentStatus);
  if (msg.recentMetrics) {
    state.metricsHistory = msg.recentMetrics;
    renderMetricsTable(msg.recentMetrics);
    renderPerformanceChart(msg.recentMetrics);
  }
  if (msg.topProcesses && msg.topProcesses.processes) {
    renderProcessTable(msg.topProcesses.processes);
  }
  fetchAppUsageSummary();
  fetchWebsiteUsageSummary();
}

// Handle Real-time Telemetry Stream Packet
function handleTelemetryFrame(msg) {
  const data = msg.data;
  if (!data) return;
  
  if (msg.agentStatus) updateAgentStatus(msg.agentStatus);
  
  // Update Hero Active App Card
  if (data.activeWindow) {
    if (DOM.currentAppTitle) DOM.currentAppTitle.textContent = data.activeWindow.title || 'Desktop / Idle';
    if (DOM.currentProcessName) DOM.currentProcessName.innerHTML = `<i data-lucide="terminal"></i> ${data.activeWindow.processName || 'explorer.exe'}`;
    if (DOM.currentPID) DOM.currentPID.innerHTML = `<i data-lucide="hash"></i> PID: ${data.activeWindow.pid || 0}`;
    if (window.lucide) lucide.createIcons();
  }

  // Update Idle Tag
  if (DOM.userIdleState) {
    if (data.isUserIdle) {
      DOM.userIdleState.innerHTML = `<i data-lucide="moon"></i> User Idle (${Math.round((data.userIdleMs || 0) / 1000)}s)`;
      DOM.userIdleState.style.borderColor = '#f59e0b';
      DOM.userIdleState.style.color = '#f59e0b';
    } else {
      DOM.userIdleState.innerHTML = `<i data-lucide="zap"></i> User Active`;
      DOM.userIdleState.style.borderColor = 'rgba(255,255,255,0.1)';
      DOM.userIdleState.style.color = '#94a3b8';
    }
    if (window.lucide) lucide.createIcons();
  }

  // Update Metric Cards
  if (DOM.valCpu) DOM.valCpu.textContent = `${data.cpuPercent || 0}%`;
  if (DOM.barCpu) DOM.barCpu.style.width = `${data.cpuPercent || 0}%`;

  if (DOM.valRam) DOM.valRam.textContent = `${data.memoryLoadPercent || 0}%`;
  if (DOM.barRam) DOM.barRam.style.width = `${data.memoryLoadPercent || 0}%`;
  if (DOM.valRamSub) DOM.valRamSub.textContent = `${((data.usedMemoryMB || 0) / 1024).toFixed(1)} / ${((data.totalMemoryMB || 0) / 1024).toFixed(1)} GB Used`;

  if (DOM.valDisk) DOM.valDisk.textContent = `${data.diskUsedPercent || 0}%`;
  if (DOM.barDisk) DOM.barDisk.style.width = `${data.diskUsedPercent || 0}%`;
  if (DOM.valDiskSub) DOM.valDiskSub.textContent = `${data.diskUsedGB || 0} / ${data.diskTotalGB || 0} GB Used`;

  if (DOM.valPower && DOM.valBatterySub) {
    if (data.powerConnected) {
      DOM.valPower.textContent = 'AC Power';
      DOM.valBatterySub.textContent = data.batteryPercent !== null ? `${data.batteryPercent}% Plugged In` : 'Plugged In';
    } else {
      DOM.valPower.textContent = 'On Battery';
      DOM.valBatterySub.textContent = `${data.batteryPercent || 0}% Remaining`;
    }
  }

  // Push to local metrics history buffer
  state.metricsHistory.push(data);
  if (state.metricsHistory.length > 60) state.metricsHistory.shift();

  renderPerformanceChart(state.metricsHistory);
  prependMetricsTableRow(data);
}

// Render Performance Line Chart
function renderPerformanceChart(metricsList) {
  if (!state.charts.perf) return;
  const labels = metricsList.map(m => new Date(m.timestamp || Date.now()).toLocaleTimeString());
  const cpuData = metricsList.map(m => m.cpuPercent || 0);
  const ramData = metricsList.map(m => m.memoryLoadPercent || 0);

  state.charts.perf.data.labels = labels;
  state.charts.perf.data.datasets[0].data = cpuData;
  state.charts.perf.data.datasets[1].data = ramData;
  state.charts.perf.update('none');
}

function handleProcessSnapshot(snapshot) {
  if (snapshot && snapshot.processes) {
    renderProcessTable(snapshot.processes);
  }
}

function renderProcessTable(processes) {
  if (!DOM.processTableBody) return;
  if (!processes || processes.length === 0) {
    DOM.processTableBody.innerHTML = `<tr><td colspan="4" class="empty-state">No active processes detected.</td></tr>`;
    return;
  }

  const totalRamMB = processes.reduce((acc, p) => acc + (p.memoryMB || 0), 0) || 1;
  let html = '';

  processes.forEach(p => {
    const memMB = p.memoryMB || 0;
    const sharePercent = ((memMB / totalRamMB) * 100).toFixed(1);
    html += `
      <tr>
        <td><code>${p.pid || 0}</code></td>
        <td><strong>${p.name || p.processName || 'Unknown'}</strong></td>
        <td>${memMB} MB</td>
        <td>
          <div style="display: flex; align-items: center; gap: 8px;">
            <div class="progress-bar-bg" style="width: 80px; margin: 0;">
              <div class="progress-bar-fill" style="width: ${sharePercent}%; background: var(--gradient-purple);"></div>
            </div>
            <span>${sharePercent}%</span>
          </div>
        </td>
      </tr>
    `;
  });

  DOM.processTableBody.innerHTML = html;
}

// Fetch App Usage Summary for Doughnut Chart
async function fetchAppUsageSummary() {
  try {
    const res = await fetch('/api/activity/summary');
    const json = await res.json();
    if (json.summary && json.summary.length > 0) {
      const labels = json.summary.map(s => s.processName);
      const data = json.summary.map(s => s.totalSeconds);
      
      if (state.charts.appPie) {
        state.charts.appPie.data.labels = labels;
        state.charts.appPie.data.datasets[0].data = data;
        state.charts.appPie.update();
      }
    }
  } catch (err) {
    console.error('Error fetching app summary:', err);
  }
}

// Fetch Website Usage Summary for Doughnut Chart
async function fetchWebsiteUsageSummary() {
  try {
    const res = await fetch('/api/activity/websites');
    const json = await res.json();
    if (json.summary && json.summary.length > 0) {
      const labels = json.summary.map(s => s.domain);
      const data = json.summary.map(s => s.totalSeconds);
      
      if (state.charts.websitePie) {
        state.charts.websitePie.data.labels = labels;
        state.charts.websitePie.data.datasets[0].data = data;
        state.charts.websitePie.update();
      }
    }
  } catch (err) {
    console.error('Error fetching website summary:', err);
  }
}

// Fetch Activity Log Table
async function fetchActivityLogs() {
  try {
    const res = await fetch('/api/activity/recent');
    const json = await res.json();
    const list = json.activity || json;
    if (Array.isArray(list) && DOM.appActivityTableBody) {
      let html = '';
      list.slice().reverse().forEach(act => {
        const timeStr = act.timestamp ? new Date(act.timestamp).toLocaleTimeString() : (act.start_time ? new Date(act.start_time).toLocaleTimeString() : 'N/A');
        const procName = act.processName || act.name || act.AppName || 'Unknown';
        const titleStr = act.title || act.Title || act.WindowTitle || '-';
        const pidNum = act.pid || act.PID || 0;
        const durSec = act.durationSeconds || act.duration_sec || act.DurationSec || 0;

        html += `
          <tr>
            <td>${timeStr}</td>
            <td><strong>${procName}</strong></td>
            <td>${titleStr}</td>
            <td><code>${pidNum}</code></td>
            <td><span class="tag">${durSec}s</span></td>
          </tr>
        `;
      });
      DOM.appActivityTableBody.innerHTML = html || `<tr><td colspan="5" class="empty-state">No window activity logs yet.</td></tr>`;
    }
  } catch (err) {
    console.error('Error fetching activity logs:', err);
  }
}

function prependMetricsTableRow(m) {
  if (!DOM.metricsTableBody) return;
  const timeStr = m.timestamp ? new Date(m.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString();
  const tr = document.createElement('tr');
  tr.innerHTML = `
    <td>${timeStr}</td>
    <td><span style="color: var(--cyan); font-weight: 600;">${m.cpuPercent || 0}%</span></td>
    <td>${((m.usedMemoryMB || 0)/1024).toFixed(1)} / ${((m.totalMemoryMB || 0)/1024).toFixed(1)} GB (${m.memoryLoadPercent || 0}%)</td>
    <td>${m.diskUsedPercent || 0}%</td>
    <td>${m.activeWindow ? m.activeWindow.processName : 'N/A'}</td>
    <td>${m.isUserIdle ? '<span style="color: var(--amber);">Idle</span>' : '<span style="color: var(--emerald);">Active</span>'}</td>
  `;
  
  if (DOM.metricsTableBody.children.length === 1 && DOM.metricsTableBody.children[0].classList.contains('empty-state')) {
    DOM.metricsTableBody.innerHTML = '';
  }
  
  DOM.metricsTableBody.insertBefore(tr, DOM.metricsTableBody.firstChild);
  if (DOM.metricsTableBody.children.length > 50) {
    DOM.metricsTableBody.removeChild(DOM.metricsTableBody.lastChild);
  }
}

function renderMetricsTable(metrics) {
  if (!DOM.metricsTableBody) return;
  DOM.metricsTableBody.innerHTML = '';
  metrics.slice(-30).reverse().forEach(m => prependMetricsTableRow(m));
}

// Agent Status Updates
function updateAgentStatus(status) {
  if (!status) return;
  const isRun = status.isRunning !== undefined ? status.isRunning : true;
  if (DOM.statusIndicator) {
    DOM.statusIndicator.className = isRun ? 'status-indicator online' : 'status-indicator offline';
  }
  if (DOM.statusText) {
    DOM.statusText.textContent = isRun ? 'Agent Active' : 'Agent Stopped';
  }
  if (DOM.btnStartAgent) DOM.btnStartAgent.disabled = isRun;
  if (DOM.btnStopAgent) DOM.btnStopAgent.disabled = !isRun;

  if (DOM.agentUptime) DOM.agentUptime.textContent = `${status.uptimeSeconds || 0}s`;
  if (DOM.agentSamples) DOM.agentSamples.textContent = status.collectedSamplesCount || 0;
}

// Control Buttons Handlers
if (DOM.btnStartAgent) {
  DOM.btnStartAgent.addEventListener('click', async () => {
    await fetch('/api/agent/start', { method: 'POST' });
    fetchTelemetryPoll();
  });
}

if (DOM.btnStopAgent) {
  DOM.btnStopAgent.addEventListener('click', async () => {
    await fetch('/api/agent/stop', { method: 'POST' });
    fetchTelemetryPoll();
  });
}

if (DOM.btnExportData) {
  DOM.btnExportData.addEventListener('click', () => {
    window.location.href = '/api/export';
  });
}

// Screenshot Gallery Functions
async function fetchScreenshots() {
  const grid = document.getElementById('screenshotGrid');
  if (!grid) return;

  try {
    const res = await fetch('/api/screenshots?limit=30');
    const json = await res.json();
    const list = Array.isArray(json) ? json : (json.screenshots || []);

    if (list.length > 0) {
      let html = '';
      list.slice().reverse().forEach(s => {
        const rawDate = s.captured_at || s.CapturedAt || s.capturedAt || s.timestamp;
        const capturedDate = rawDate ? new Date(rawDate).toLocaleTimeString() : 'N/A';
        const filePath = s.file_path || s.FilePath || s.filePath || "";
        const imgSrc = s.urlPath || (filePath ? `/api/screenshots/image?path=${encodeURIComponent(filePath)}` : '');
        const appTitle = s.activeApp || s.active_app || 'Desktop Capture';
        const winTitle = s.windowTitle || s.window_title || s.file_name || '-';

        html += `
          <div class="screenshot-card">
            <div class="screenshot-img-wrap">
              <img src="${imgSrc}" alt="Screen capture" onclick="window.open('${imgSrc}', '_blank')" />
            </div>
            <div class="screenshot-meta">
              <span class="time">${capturedDate}</span>
              <div class="app-title">${appTitle}</div>
              <div class="window-title">${winTitle}</div>
            </div>
          </div>
        `;
      });
      grid.innerHTML = html;
    } else {
      grid.innerHTML = `<div class="empty-state">No screen captures recorded yet.</div>`;
    }
  } catch (err) {
    console.error('Error fetching screenshots:', err);
  }
}

// Dropdown handler for Automatic Screenshot Interval Frequency
const selectScreenshotInterval = document.getElementById('selectScreenshotInterval');
if (selectScreenshotInterval) {
  selectScreenshotInterval.addEventListener('change', async (e) => {
    const selectedMs = Number(e.target.value);
    const selectedSec = Math.round(selectedMs / 1000);
    try {
      await fetch('/api/screenshots/interval', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ interval_sec: selectedSec })
      });
    } catch (err) {
      console.error('Error changing screenshot interval:', err);
    }
  });
}

// Periodic background fetches
setInterval(fetchAppUsageSummary, 5000);
setInterval(fetchWebsiteUsageSummary, 5000);
setInterval(fetchActivityLogs, 5000);
setInterval(fetchScreenshots, 10000);

// Initialize on Load
initCharts();
connectWebSocket();
fetchActivityLogs();
fetchAppUsageSummary();
fetchWebsiteUsageSummary();
fetchScreenshots();
