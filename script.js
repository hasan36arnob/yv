const API_URL = '/api';

const state = {
	user: null,
	token: localStorage.getItem('todopro_token'),
	refreshToken: localStorage.getItem('todopro_refresh'),
	workspace: null,
	workspaces: [],
	tasks: [],
	ws: null,
	wsConnected: false,
	currentView: 'tasks',
	filter: 'all'
};

document.addEventListener('DOMContentLoaded', async () => {
	await initAuth();
	await initApp();
	setupEventListeners();
	await checkAPIHealth();
});

async function initAuth() {
	if (state.token) {
		try {
			const user = await fetchProfile();
			state.user = user;
			updateAuthUI(true);
			await loadWorkspaces();
		} catch (error) {
			clearTokens();
			showAuthModals();
		}
	} else {
		showAuthModals();
	}
}

async function initApp() {
	const mobileMenuBtn = document.getElementById('mobile-menu-btn');
	if (mobileMenuBtn) {
		mobileMenuBtn.addEventListener('click', toggleMobileMenu);
	}
}

async function checkAPIHealth() {
	try {
		const response = await fetch(`${API_URL}/profile`, {
			headers: { 'Authorization': `Bearer ${state.token}` }
		});
		return response.ok;
	} catch (error) {
		return false;
	}
}

async function register(email, password, firstName, lastName) {
	const response = await fetch(`${API_URL}/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password, first_name: firstName, last_name: lastName })
	});
	const data = await response.json();
	if (!response.ok) throw new Error(data.error || 'Registration failed');
	
	state.token = data.token;
	state.refreshToken = data.refresh_token;
	state.user = data.user;
	saveTokens();
	updateAuthUI(true);
	hideAuthModals();
	await loadWorkspaces();
	return data;
}

async function login(email, password) {
	const response = await fetch(`${API_URL}/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password })
	});
	const data = await response.json();
	if (!response.ok) throw new Error(data.error || 'Login failed');
	
	state.token = data.token;
	state.refreshToken = data.refresh_token;
	state.user = data.user;
	saveTokens();
	updateAuthUI(true);
	hideAuthModals();
	await loadWorkspaces();
	return data;
}

async function logout() {
	state.token = null;
	state.refreshToken = null;
	state.user = null;
	state.workspace = null;
	state.tasks = [];
	clearTokens();
	disconnectWebSocket();
	updateAuthUI(false);
	showAuthModals();
}

function saveTokens() {
	if (state.token) localStorage.setItem('todopro_token', state.token);
	if (state.refreshToken) localStorage.setItem('todopro_refresh', state.refreshToken);
}

function clearTokens() {
	localStorage.removeItem('todopro_token');
	localStorage.removeItem('todopro_refresh');
}

async function fetchProfile() {
	const response = await fetch(`${API_URL}/profile`, {
		headers: { 'Authorization': `Bearer ${state.token}` }
	});
	if (!response.ok) throw new Error('Failed to fetch profile');
	return response.json();
}

async function apiRequest(endpoint, method = 'GET', body = null) {
	const options = {
		method,
		headers: {
			'Content-Type': 'application/json',
			'Authorization': `Bearer ${state.token}`
		}
	};

	if (body) {
		options.body = JSON.stringify(body);
	}

	let response = await fetch(`${API_URL}${endpoint}`, options);

	if (response.status === 401 && state.refreshToken) {
		const refreshed = await refreshAuthToken();
		if (refreshed) {
			options.headers.Authorization = `Bearer ${state.token}`;
			response = await fetch(`${API_URL}${endpoint}`, options);
		}
	}

	const data = await response.json();
	if (!response.ok) {
		throw new Error(data.error || 'API request failed');
	}
	return data;
}

async function loadWorkspaces() {
	try {
		state.workspaces = await apiRequest('/workspaces');
		if (state.workspaces.length > 0) {
			selectWorkspace(state.workspaces[0].id);
		} else {
			await createWorkspace('My Workspace', 'Personal task management');
			await loadWorkspaces();
		}
	} catch (error) {
		console.error('Failed to load workspaces:', error);
	}
}

async function createWorkspace(name, description) {
	const workspace = await apiRequest('/workspaces/create', 'POST', { name, description });
	state.workspaces.push(workspace);
	selectWorkspace(workspace.id);
	showNotification('Workspace created', 'success');
}

function selectWorkspace(workspaceId) {
	state.workspace = state.workspaces.find(w => w.id === workspaceId);
	loadTasks();
	connectWebSocket();
	updateWorkspaceUI();
}

async function loadTasks() {
	if (!state.workspace) return;
	try {
		state.tasks = await apiRequest(`/tasks?workspace_id=${state.workspace.id}`);
		renderTasks();
		updateTaskCounts();
	} catch (error) {
		console.error('Failed to load tasks:', error);
	}
}

async function createTask(title, description = '', priority = 'medium', assigneeId = null, dueDate = null) {
	const task = await apiRequest(`/tasks/create?workspace_id=${state.workspace.id}`, 'POST', {
		title, description, priority, assignee_id: assigneeId, due_date: dueDate
	});
	state.tasks.unshift(task);
	renderTasks();
	updateTaskCounts();
}

async function updateTask(taskId, updates) {
	const task = await apiRequest(`/tasks/update?id=${taskId}`, 'PUT', updates);
	const index = state.tasks.findIndex(t => t.id === taskId);
	if (index !== -1) state.tasks[index] = task;
	renderTasks();
	updateTaskCounts();
}

async function deleteTask(taskId) {
	await apiRequest(`/tasks/delete?id=${taskId}`, 'DELETE');
	state.tasks = state.tasks.filter(t => t.id !== taskId);
	renderTasks();
	updateTaskCounts();
}

function connectWebSocket() {
	if (state.ws) disconnectWebSocket();

	const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	const wsUrl = `${wsProtocol}//${window.location.host}/ws?workspace_id=${state.workspace.id}`;
	
	state.ws = new WebSocket(wsUrl, `Bearer ${state.token}`);
	state.ws.onopen = () => { state.wsConnected = true; };
	state.ws.onmessage = (event) => {
		const message = JSON.parse(event.data);
		handleWebSocketMessage(message);
	};
	state.ws.onclose = () => {
		state.wsConnected = false;
		setTimeout(connectWebSocket, 5000);
	};
}

function disconnectWebSocket() {
	if (state.ws) {
		state.ws.close();
		state.ws = null;
	}
}

function handleWebSocketMessage(message) {
	switch (message.type) {
		case 'task_created':
			if (!state.tasks.find(t => t.id === message.payload.id)) {
				state.tasks.unshift(message.payload);
				renderTasks();
				updateTaskCounts();
			}
			break;
		case 'task_updated':
			const idx = state.tasks.findIndex(t => t.id === message.payload.id);
			if (idx !== -1) state.tasks[idx] = message.payload;
			else state.tasks.push(message.payload);
			renderTasks();
			updateTaskCounts();
			break;
		case 'task_deleted':
			state.tasks = state.tasks.filter(t => t.id !== message.payload.id);
			renderTasks();
			updateTaskCounts();
			break;
	}
}

function renderTasks() {
	const todoList = document.getElementById('todoList');
	if (!todoList) return;

	const filtered = state.tasks.filter(task => {
		if (state.filter === 'active') return task.status === 'pending';
		if (state.filter === 'completed') return task.status === 'completed';
		return true;
	});

	if (filtered.length === 0) {
		todoList.innerHTML = `<li class="empty-state">${state.filter === 'all' ? 'No tasks yet. Add one below.' : 'No tasks in this filter.'}</li>`;
		return;
	}

	todoList.innerHTML = filtered.map(task => `
		<li class="task-item ${task.status === 'completed' ? 'completed' : ''}" data-id="${task.id}">
			<div class="task-content">
				<div class="task-header">
					<strong>${escapeHtml(task.title)}</strong>
					<span class="task-priority priority-${task.priority}">${task.priority}</span>
				</div>
				${task.description ? `<p class="task-description">${escapeHtml(task.description)}</p>` : ''}
				<div class="task-meta">
					<span class="task-assignee">${task.assignee ? task.assignee.first_name + ' ' + task.assignee.last_name : 'Unassigned'}</span>
					${task.due_date ? `<span class="task-due">Due: ${formatDate(task.due_date)}</span>` : ''}
				</div>
			</div>
			<div class="task-actions">
				<button onclick="toggleTaskStatus(${task.id}, '${task.status}')" class="btn-icon ${task.status === 'completed' ? 'btn-undo' : 'btn-complete'}">
					${task.status === 'completed' ? '↩' : '✓'}
				</button>
				<button onclick="promptDeleteTask(${task.id})" class="btn-icon btn-delete">🗑</button>
			</div>
		</li>
	`).join('');
}

function updateTaskCounts() {
	const taskCount = document.getElementById('task-count');
	const completedCount = document.getElementById('completed-count');
	if (taskCount) taskCount.textContent = state.tasks.length;
	if (completedCount) completedCount.textContent = state.tasks.filter(t => t.status === 'completed').length;
}

function updateAuthUI(isLoggedIn) {
	const userSection = document.getElementById('user-section');
	const appSection = document.getElementById('app-section');
	
	if (isLoggedIn && state.user) {
		if (userSection) {
			userSection.innerHTML = `
				<div class="user-menu">
					<button id="workspace-switcher" class="btn-secondary">${state.workspace ? state.workspace.name : 'Select Workspace'}</button>
					<button id="user-profile" class="btn-secondary">${state.user.first_name || state.user.email}</button>
					<button id="logout-btn" class="btn-outline">Logout</button>
				</div>
			`;
			setupUserMenuListeners();
		}
		if (appSection) appSection.style.display = 'block';
	} else {
		if (appSection) appSection.style.display = 'none';
	}
}

function updateWorkspaceUI() {
	const workspaceName = document.getElementById('workspace-name');
	if (workspaceName && state.workspace) {
		workspaceName.textContent = state.workspace.name;
	}
}

function setupEventListeners() {
	const todoInput = document.getElementById('todoInput');
	const addBtn = document.getElementById('addBtn');
	
	if (todoInput && addBtn) {
		addBtn.addEventListener('click', handleAddTask);
		todoInput.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') handleAddTask();
		});
	}

	['all', 'active', 'completed'].forEach(filter => {
		const btn = document.getElementById(`filter-${filter}`);
		if (btn) {
			btn.addEventListener('click', () => setFilter(filter));
		}
	});
}

function setupUserMenuListeners() {
	const logoutBtn = document.getElementById('logout-btn');
	if (logoutBtn) {
		logoutBtn.addEventListener('click', logout);
	}
}

async function handleAddTask() {
	const input = document.getElementById('todoInput');
	const text = input.value.trim();
	if (!text) return;
	if (state.workspace) {
		await createTask(text);
		input.value = '';
	}
}

async function toggleTaskStatus(taskId, currentStatus) {
	await updateTask(taskId, { 
		status: currentStatus === 'completed' ? 'pending' : 'completed',
		completed_at: currentStatus === 'completed' ? null : new Date().toISOString()
	});
}

async function promptDeleteTask(taskId) {
	if (confirm('Delete this task?')) {
		await deleteTask(taskId);
	}
}

function setFilter(filter) {
	state.filter = filter;
	renderTasks();
}

function showAuthModals(initialTab = 'login') {
	const overlay = document.getElementById('auth-overlay');
	if (overlay) {
		overlay.style.display = 'flex';
		showAuthTab(initialTab);
	}
}

function hideAuthModals() {
	const overlay = document.getElementById('auth-overlay');
	if (overlay) overlay.style.display = 'none';
}

function showAuthTab(tab) {
	const loginForm = document.getElementById('login-form');
	const registerForm = document.getElementById('register-form');
	if (tab === 'login' && loginForm) {
		loginForm.style.display = 'block';
		registerForm.style.display = 'none';
	} else if (registerForm) {
		loginForm.style.display = 'none';
		registerForm.style.display = 'block';
	}
}

async function handleLogin(e) {
	e.preventDefault();
	const email = document.getElementById('login-email').value;
	const password = document.getElementById('login-password').value;
	try {
		await login(email, password);
		hideAuthModals();
	} catch (error) {
		showAuthError('login-error', error.message);
	}
}

async function handleRegister(e) {
	e.preventDefault();
	const email = document.getElementById('register-email').value;
	const password = document.getElementById('register-password').value;
	const firstName = document.getElementById('register-firstname').value;
	const lastName = document.getElementById('register-lastname').value;
	
	if (password.length < 8) {
		showAuthError('register-error', 'Password must be at least 8 characters');
		return;
	}
	
	try {
		await register(email, password, firstName, lastName);
		hideAuthModals();
	} catch (error) {
		showAuthError('register-error', error.message);
	}
}

function showAuthError(elementId, message) {
	const errorEl = document.getElementById(elementId);
	if (errorEl) {
		errorEl.textContent = message;
		errorEl.style.display = 'block';
	}
}

function showPaymentModal() {
	const modal = document.createElement('div');
	modal.className = 'modal-overlay';
	modal.innerHTML = `
		<div class="modal payment-modal">
			<button class="close-btn" onclick="this.closest('.modal-overlay').remove()">×</button>
			<h2>Upgrade to Pro</h2>
			<div class="payment-instructions">
				<p class="payment-amount">Send <strong>৳500</strong> to bKash:</p>
				<div class="payment-number">017XXXXXXXX</div>
				<p class="payment-note">(Personal bKash/Nagad number)</p>
			</div>
			<form id="payment-form" class="payment-form">
				<div class="form-group">
					<label for="trx-id">Transaction ID</label>
					<input type="text" id="trx-id" placeholder="Enter bKash/Nagad TrxID" required>
				</div>
				<button type="submit" class="btn-primary btn-block">Submit for Approval</button>
			</form>
			<p id="payment-message" class="payment-message"></p>
		</div>
	`;
	
	document.body.appendChild(modal);
	
	document.getElementById('payment-form').addEventListener('submit', async (e) => {
		e.preventDefault();
		const trxId = document.getElementById('trx-id').value.trim();
		const messageEl = document.getElementById('payment-message');
		
		try {
			const response = await fetch(`${API_URL}/payments/submit`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${state.token}`
				},
				body: JSON.stringify({ trx_id: trxId, method: 'bkash' })
			});
			
			const data = await response.json();
			if (response.ok) {
				messageEl.innerHTML = '<span class="status-badge status-pending">Payment pending. Admin will approve shortly.</span>';
				document.getElementById('payment-form').reset();
			} else {
				messageEl.innerHTML = `<span class="status-badge status-error">${data.error}</span>`;
			}
		} catch (error) {
			messageEl.innerHTML = `<span class="status-badge status-error">Submission failed</span>`;
		}
	});
}

function formatDate(dateStr) {
	const date = new Date(dateStr);
	return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

function escapeHtml(text) {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}

function showNotification(message, type = 'info') {
	const toast = document.createElement('div');
	toast.className = `toast toast-${type}`;
	toast.textContent = message;
	document.body.appendChild(toast);
	setTimeout(() => toast.remove(), 3000);
}

function toggleMobileMenu() {
	const menuBtn = document.getElementById('mobile-menu-btn');
	const navbar = document.querySelector('.navbar');
	const expanded = menuBtn.getAttribute('aria-expanded') === 'true';
	menuBtn.setAttribute('aria-expanded', !expanded);
	navbar.classList.toggle('open');
}

document.showPaymentModal = showPaymentModal;