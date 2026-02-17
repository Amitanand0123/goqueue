// WebSocket connection for real-time updates
let ws = null;

function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = function() {
        console.log('WebSocket connected');
    };

    ws.onmessage = function(event) {
        try {
            const data = JSON.parse(event.data);
            handleEvent(data);
        } catch (e) {
            console.error('Failed to parse WebSocket message:', e);
        }
    };

    ws.onclose = function() {
        console.log('WebSocket disconnected, reconnecting in 3s...');
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = function(err) {
        console.error('WebSocket error:', err);
        ws.close();
    };
}

function handleEvent(data) {
    const event = data.event;
    let message = '';
    let color = 'teal';

    switch (event) {
        case 'job:created':
            message = `Job created: ${data.type} (${data.priority})`;
            color = 'blue';
            break;
        case 'job:started':
            message = `Job started: ${data.job_id.substring(0, 8)}... on ${data.worker_id}`;
            color = 'blue';
            break;
        case 'job:completed':
            message = `Job completed: ${data.job_id.substring(0, 8)}... (${data.duration_ms}ms)`;
            color = 'green';
            break;
        case 'job:failed':
            message = `Job failed: ${data.job_id.substring(0, 8)}... (retry ${data.retry_count})`;
            color = 'orange';
            break;
        case 'job:dead':
            message = `Job dead: ${data.job_id.substring(0, 8)}...`;
            color = 'red';
            break;
        default:
            return;
    }

    showToast(message, color);
}

function showToast(message, color) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `bg-gray-800 border border-gray-700 rounded-lg shadow-lg p-4 flex items-center gap-3 animate-slide-in`;
    toast.innerHTML = `
        <div class="w-2 h-2 rounded-full bg-${color}-400"></div>
        <div class="text-sm text-gray-200">${message}</div>
    `;

    container.appendChild(toast);

    // Auto-remove after 5 seconds
    setTimeout(() => {
        toast.classList.add('animate-fade-out');
        setTimeout(() => toast.remove(), 300);
    }, 5000);
}

// Chart.js initialization
function initCharts(stats) {
    if (!stats) return;

    // Queue sizes bar chart
    const queueCtx = document.getElementById('queueChart');
    if (queueCtx) {
        const queues = stats.queues || {};
        new Chart(queueCtx, {
            type: 'bar',
            data: {
                labels: ['Critical', 'High', 'Default', 'Low'],
                datasets: [{
                    label: 'Pending Jobs',
                    data: [
                        queues.critical || 0,
                        queues.high || 0,
                        queues['default'] || 0,
                        queues.low || 0
                    ],
                    backgroundColor: [
                        'rgba(239, 68, 68, 0.5)',
                        'rgba(249, 115, 22, 0.5)',
                        'rgba(20, 184, 166, 0.5)',
                        'rgba(107, 114, 128, 0.5)'
                    ],
                    borderColor: [
                        'rgb(239, 68, 68)',
                        'rgb(249, 115, 22)',
                        'rgb(20, 184, 166)',
                        'rgb(107, 114, 128)'
                    ],
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: {
                    y: {
                        beginAtZero: true,
                        ticks: { color: '#9ca3af' },
                        grid: { color: 'rgba(55, 65, 81, 0.3)' }
                    },
                    x: {
                        ticks: { color: '#9ca3af' },
                        grid: { display: false }
                    }
                }
            }
        });
    }

    // Processed line chart (placeholder data)
    const processedCtx = document.getElementById('processedChart');
    if (processedCtx) {
        const hours = [];
        const values = [];
        for (let i = 23; i >= 0; i--) {
            const h = new Date();
            h.setHours(h.getHours() - i);
            hours.push(h.getHours() + ':00');
            values.push(0); // Placeholder — would need hourly metrics
        }

        new Chart(processedCtx, {
            type: 'line',
            data: {
                labels: hours,
                datasets: [{
                    label: 'Processed',
                    data: values,
                    borderColor: 'rgb(20, 184, 166)',
                    backgroundColor: 'rgba(20, 184, 166, 0.1)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 0
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: {
                    y: {
                        beginAtZero: true,
                        ticks: { color: '#9ca3af' },
                        grid: { color: 'rgba(55, 65, 81, 0.3)' }
                    },
                    x: {
                        ticks: { color: '#9ca3af', maxTicksLimit: 8 },
                        grid: { display: false }
                    }
                }
            }
        });
    }
}

// Connect WebSocket on page load
document.addEventListener('DOMContentLoaded', function() {
    connectWebSocket();
});
