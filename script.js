// TodoPro SaaS - Frontend Application
const API_URL = '/api';
const APP_URL = window.location.origin;

// State management
const state = {
	user: null,
	token: localStorage.getItem('todopro_token'),
	refreshToken: localStorage.getItem('todopro_refresh'),
	workspace: null,
	workspaces: [],
	tasks: [],
	ws: null,
	wsConnected: false,
	currentView: 'tasks', // tasks, dashboard, analytics
	filter: 'all'
};

// Stripe public key
const STRIPE_PUBLIC_KEY = 'pk_test_placeholder'; // Replace with actual Stripe public key

// ============================================
// INITIALIZATION
// ============================================

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
			console.log('Token invalid, clearing...');
			clearTokens();
			showAuthModals();
		}
	} else {
		showAuthModals();
	}
}

async function initApp() {
	// Initialize pricing toggle
	applyPricingMode();
	
	// Initialize mobile menu
	const mobileMenuBtn = document.getElementById('mobile-menu-btn');
	if (mobileMenuBtn) {
		mobileMenuBtn.addEventListener('click', toggleMobileMenu);
	}
	
	// Initialize contact form
	const contactForm = document.getElementById('contactForm');
	if (contactForm) {
		contactForm.addEventListener('submit', submitContactForm);
	}
}

async function checkAPIHealth() {
	try {
		const response = await fetch(`${API_URL}/profile`, {
			headers: { 'Authorization': `Bearer ${state.token}` }
		});
		if (response.ok) {
			return true;
		}
	} catch (error) {
		console.log('Backend API not available');
	}
	return false;
}

// ============================================
// AUTHENTICATION
// ============================================

async function register(email, password, firstName, lastName) {
	try {
		const response = await fetch(`${API_URL}/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ email, password, first_name: firstName, last_name: lastName })
		});

		const data = await response.json();

		if (!response.ok) {
			throw new Error(data.error || 'Registration failed');
		}

		// Store tokens
		state.token = data.token;
		state.refreshToken = data.refresh_token;
		state.user = data.user;
		saveTokens();
		
		updateAuthUI(true);
		hideAuthModals();
		await loadWorkspaces();
		
		return data;
	} catch (error) {
		console.error('Registration error:', error);
		throw error;
	}
}

async function login(email, password) {
	try {
		const response = await fetch(`${API_URL}/login`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ email, password })
		});

		const data = await response.json();

		if (!response.ok) {
			throw new Error(data.error || 'Login failed');
		}

		state.token = data.token;
		state.refreshToken = data.refresh_token;
		state.user = data.user;
		saveTokens();
		
		updateAuthUI(true);
		hideAuthModals();
		await loadWorkspaces();
		
		return data;
	} catch (error) {
		console.error('Login error:', error);
		throw error;
	}
}

async function logout() {
	try {
		state.token = null;
		state.refreshToken = null;
		state.user = null;
		state.workspace = null;
		state.tasks = [];
		
		clearTokens();
		disconnectWebSocket();
		updateAuthUI(false);
		showAuthModals();
	} catch (error) {
		console.error('Logout error:', error);
	}
}

async function refreshAuthToken() {
	if (!state.refreshToken) return false;

	try {
		const response = await fetch(`${API_URL}/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refresh_token: state.refreshToken })
		});

		if (!response.ok) {
			throw new Error('Refresh failed');
		}

		const data = await response.json();
		state.token = data.token;
		state.refreshToken = data.refresh_token;
		saveTokens();
		return true;
	} catch (error) {
		console.error('Token refresh failed:', error);
		clearTokens();
		return false;
	}
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

// ============================================
// API HELPERS
// ============================================

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

	// Handle 401 (token expired)
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

// ============================================
// WORKSPACE MANAGEMENT
// ============================================

async function loadWorkspaces() {
	try {
		state.workspaces = await apiRequest('/workspaces');
		
		// Select first workspace or create default
		if (state.workspaces.length > 0) {
			selectWorkspace(state.workspaces[0].id);
		} else {
			// Create default personal workspace
			await createWorkspace('My Workspace', 'Personal task management');
			await loadWorkspaces();
		}
	} catch (error) {
		console.error('Failed to load workspaces:', error);
	}
}

async function createWorkspace(name, description) {
	try {
		const workspace = await apiRequest('/workspaces/create', 'POST', { name, description });
		state.workspaces.push(workspace);
		selectWorkspace(workspace.id);
		showNotification('Workspace created successfully', 'success');
	} catch (error) {
		console.error('Failed to create workspace:', error);
		throw error;
	}
}

function selectWorkspace(workspaceId) {
	state.workspace = state.workspaces.find(w => w.id === workspaceId);
	loadTasks();
	connectWebSocket();
	updateWorkspaceUI();
}

// ============================================
// TASK MANAGEMENT
// ============================================

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
	try {
		const task = await apiRequest(`/tasks/create?workspace_id=${state.workspace.id}`, 'POST', {
			title,
			description,
			priority,
			assignee_id: assigneeId,
			due_date: dueDate
		});
		
		state.tasks.unshift(task);
		renderTasks();
		updateTaskCounts();
		showNotification('Task created', 'success');
	} catch (error) {
		console.error('Failed to create task:', error);
		showNotification('Failed to create task', 'error');
	}
}

async function updateTask(taskId, updates) {
	try {
		const task = await apiRequest(`/tasks/update?id=${taskId}`, 'PUT', updates);
		
		const index = state.tasks.findIndex(t => t.id === taskId);
		if (index !== -1) {
			state.tasks[index] = task;
		}
		
		renderTasks();
		updateTaskCounts();
	} catch (error) {
		console.error('Failed to update task:', error);
		showNotification('Failed to update task', 'error');
	}
}

async function deleteTask(taskId) {
	try {
		await apiRequest(`/tasks/delete?id=${taskId}`, 'DELETE');
		state.tasks = state.tasks.filter(t => t.id !== taskId);
		renderTasks();
		updateTaskCounts();
		showNotification('Task deleted', 'success');
	} catch (error) {
		console.error('Failed to delete task:', error);
		showNotification('Failed to delete task', 'error');
	}
}

// ============================================
// WEBSOCKET
// ============================================

function connectWebSocket() {
	if (state.ws) {
		disconnectWebSocket();
	}

	const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	const wsUrl = `${wsProtocol}//${window.location.host}/ws?workspace_id=${state.workspace.id}`;
	
	state.ws = new WebSocket(wsUrl, `Bearer ${state.token}`);

	state.ws.onopen = () => {
		console.log('WebSocket connected');
		state.wsConnected = true;
	};

	state.ws.onmessage = (event) => {
		const message = JSON.parse(event.data);
		handleWebSocketMessage(message);
	};

	state.ws.onclose = () => {
		console.log('WebSocket disconnected');
		state.wsConnected = false;
		// Reconnect after 5 seconds
		setTimeout(connectWebSocket, 5000);
	};

	state.ws.onerror = (error) => {
		console.error('WebSocket error:', error);
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
			if (idx !== -1) {
				state.tasks[idx] = message.payload;
			} else {
				state.tasks.push(message.payload);
			}
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

// ============================================
// STRIPE PAYMENTS
// ============================================

async function createCheckoutSession(planId) {
	if (!state.user) {
		showAuthModals('login');
		return;
	}

	try {
		// Load Stripe.js
		if (!window.Stripe) {
			const script = document.createElement('script');
			script.src = 'https://js.stripe.com/v3/';
			script.onload = () => initStripeCheckout(planId);
			document.head.appendChild(script);
		} else {
			initStripeCheckout(planId);
		}
	} catch (error) {
		console.error('Stripe checkout error:', error);
		showNotification('Payment system unavailable', 'error');
	}
}

function initStripeCheckout(planId) {
	const stripe = Stripe(STRIPE_PUBLIC_KEY);
	
	// Create checkout session
	fetch(`${API_URL}/checkout/create`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'Authorization': `Bearer ${state.token}`
		},
		body: JSON.stringify({ plan_id: planId })
	})
	.then(res => res.json())
	.then(data => {
		if (data.url) {
			window.location.href = data.url;
		} else {
			throw new Error('Invalid checkout response');
		}
	})
	.catch(error => {
		console.error('Checkout error:', error);
		showNotification('Failed to start checkout', 'error');
	});
}

// ============================================
// ANALYTICS & DASHBOARD
// ============================================

async function loadAnalytics() {
	if (!state.workspace) return;
	
	try {
		const analytics = await apiRequest(`/analytics?workspace_id=${state.workspace.id}`);
		renderAnalytics(analytics);
	} catch (error) {
		console.error('Failed to load analytics:', error);
	}
}

function renderAnalytics(data) {
	// Render charts (integrate Chart.js here)
	const completionRate = document.getElementById('completion-rate');
	if (completionRate) {
		completionRate.textContent = `${data.summary.completion_rate.toFixed(1)}%`;
	}
	
	// Populate team productivity table
	const tbody = document.getElementById('productivity-table');
	if (tbody && data.by_assignee) {
		tbody.innerHTML = data.by_assignee.map(member => `
			<tr>
				<td>${member.user.first_name} ${member.user.last_name}</td>
				<td>${member.total}</td>
				<td>${member.completed}</td>
				<td>${member.in_progress}</td>
				<td>${calculateRate(member.completed, member.total).toFixed(1)}%</td>
			</tr>
		`).join('');
	}
}

// ============================================
// UI RENDERING
// ============================================

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
				<button onclick="toggleTaskStatus(${task.id}, '${task.status}')" 
					class="btn-icon ${task.status === 'completed' ? 'btn-undo' : 'btn-complete'}">
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
	const authSection = document.getElementById('auth-section');
	const appSection = document.getElementById('app-section');
	
	if (isLoggedIn && state.user) {
		if (userSection) {
			userSection.innerHTML = `
				<div class="user-menu">
					<button id="workspace-switcher" class="btn-secondary">
						${state.workspace ? state.workspace.name : 'Select Workspace'}
					</button>
					<button id="user-profile" class="btn-secondary">
						${state.user.first_name || state.user.email}
					</button>
					<button id="logout-btn" class="btn-outline">Logout</button>
				</div>
			`;
			setupUserMenuListeners();
		}
		if (authSection) authSection.style.display = 'none';
		if (appSection) appSection.style.display = 'block';
	} else {
		if (authSection) authSection.style.display = 'flex';
		if (appSection) appSection.style.display = 'none';
	}
}

function updateWorkspaceUI() {
	const workspaceName = document.getElementById('workspace-name');
	if (workspaceName && state.workspace) {
		workspaceName.textContent = state.workspace.name;
	}
}

// ============================================
// EVENT LISTENERS
// ============================================

function setupEventListeners() {
	// Task input
	const todoInput = document.getElementById('todoInput');
	const addBtn = document.getElementById('addBtn');
	
	if (todoInput && addBtn) {
		addBtn.addEventListener('click', handleAddTask);
		todoInput.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') handleAddTask();
		});
	}

	// Filters
	['all', 'active', 'completed'].forEach(filter => {
		const btn = document.getElementById(`filter-${filter}`);
		if (btn) {
			btn.addEventListener('click', () => setFilter(filter));
		}
	});

	// Clear completed
	const clearCompleted = document.getElementById('clear-completed');
	if (clearCompleted) {
		clearCompleted.addEventListener('click', clearCompletedTasks);
	}

	// Pricing toggle
	const pricingToggle = document.getElementById('pricing-toggle');
	if (pricingToggle) {
		pricingToggle.addEventListener('change', applyPricingMode);
	}

	// Logout
	const logoutBtn = document.getElementById('logout-btn');
	if (logoutBtn) {
		logoutBtn.addEventListener('click', logout);
	}
}

function setupUserMenuListeners() {
	const logoutBtn = document.getElementById('logout-btn');
	if (logoutBtn) {
		logoutBtn.addEventListener('click', logout);
	}
	
	const profileBtn = document.getElementById('user-profile');
	if (profileBtn) {
		profileBtn.addEventListener('click', () => {
			// Navigate to profile page
			showNotification('Profile page coming soon', 'info');
		});
	}
	
	const workspaceSwitcher = document.getElementById('workspace-switcher');
	if (workspaceSwitcher) {
		workspaceSwitcher.addEventListener('click', () => {
			showWorkspaceSwitcher();
		});
	}
}

// ============================================
// TASK OPERATIONS
// ============================================

async function handleAddTask() {
	const input = document.getElementById('todoInput');
	const text = input.value.trim();
	
	if (!text) return;
	
	if (state.workspace) {
		await createTask(text);
		input.value = '';
	} else {
		showNotification('Please select a workspace first', 'warning');
	}
}

async function toggleTaskStatus(taskId, currentStatus) {
	const updates = { 
		status: currentStatus === 'completed' ? 'pending' : 'completed',
		completed_at: currentStatus === 'completed' ? null : new Date().toISOString()
	};
	await updateTask(taskId, updates);
}

async function promptDeleteTask(taskId) {
	if (confirm('Are you sure you want to delete this task?')) {
		await deleteTask(taskId);
	}
}

function clearCompletedTasks() {
	const completedTasks = state.tasks.filter(t => t.status === 'completed');
	if (completedTasks.length === 0) return;
	
	if (confirm(`Delete ${completedTasks.length} completed task(s)?`)) {
		completedTasks.forEach(task => deleteTask(task.id));
	}
}

function setFilter(filter) {
	state.filter = filter;
	['filter-all', 'filter-active', 'filter-completed'].forEach(id => {
		const btn = document.getElementById(id);
		if (btn) btn.classList.toggle('active', id === `filter-${filter}`);
	});
	renderTasks();
}

// ============================================
// AUTH MODALS
// ============================================

function showAuthModals(initialTab = 'login') {
	const overlay = document.getElementById('auth-overlay');
	if (overlay) {
		overlay.style.display = 'flex';
		showAuthTab(initialTab);
	}
}

function hideAuthModals() {
	const overlay = document.getElementById('auth-overlay');
	if (overlay) {
		overlay.style.display = 'none';
	}
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

// ============================================
// PRICING & PAYMENTS
// ============================================

function applyPricingMode() {
	const toggle = document.getElementById('pricing-toggle');
	if (!toggle) return;
	
	const isYearly = toggle.checked;
	const plans = [
		{ selector: '.pricing-card:nth-child(1) .amount', monthly: 4, yearly: 40 },
		{ selector: '.pricing-card:nth-child(2) .amount', monthly: 9, yearly: 90 },
		{ selector: '.pricing-card:nth-child(3) .amount', monthly: 15, yearly: 150 }
	];
	
	plans.forEach(plan => {
		const el = document.querySelector(plan.selector);
		if (el) el.textContent = isYearly ? plan.yearly : plan.monthly;
	});
}

async function handleSubscribe(planId) {
	await createCheckoutSession(planId);
}

// ============================================
// UTILITIES
// ============================================

function formatDate(dateStr) {
	const date = new Date(dateStr);
	return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

function escapeHtml(text) {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}

function calculateRate(completed, total) {
	if (total === 0) return 0;
	return (completed / total * 100).toFixed(1);
}

function showNotification(message, type = 'info') {
	// Create toast notification
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

async function submitContactForm(e) {
	e.preventDefault();
	const name = document.getElementById('name').value.trim();
	const email = document.getElementById('email').value.trim();
	const message = document.getElementById('message').value.trim();
	
	if (!name || !email || !message) {
		showFormError('Please fill in all required fields');
		return;
	}
	
	// Send message (implement endpoint)
	showNotification('Message sent! We will reply within one business day.', 'success');
	e.target.reset();
}

function showFormError(message) {
	const statusEl = document.getElementById('contactMessage');
	if (statusEl) {
		statusEl.textContent = message;
		statusEl.style.color = '#dc2626';
	}
}

// ============================================
// WORKSPACE SWITCHER MODAL
// ============================================

function showWorkspaceSwitcher() {
	// Create modal
	const modal = document.createElement('div');
	modal.className = 'modal-overlay';
	modal.innerHTML = `
		<div class="modal">
			<h2>Select Workspace</h2>
			<div class="workspace-list">
				${state.workspaces.map(ws => `
					<button class="workspace-option ${state.workspace && state.workspace.id === ws.id ? 'active' : ''}" 
						data-id="${ws.id}">
						${ws.name}
						${state.workspace && state.workspace.id === ws.id ? ' ✓' : ''}
					</button>
				`).join('')}
			</div>
			<button id="create-workspace-btn" class="btn-primary">+ New Workspace</button>
			<button class="btn-secondary modal-close">Cancel</button>
		</div>
	`;
	
	document.body.appendChild(modal);
	
	// Event listeners
	modal.querySelectorAll('.workspace-option').forEach(btn => {
		btn.addEventListener('click', () => {
			const wsId = parseInt(btn.dataset.id);
			selectWorkspace(wsId);
			modal.remove();
		});
	});
	
	modal.querySelector('#create-workspace-btn').addEventListener('click', () => {
		promptCreateWorkspace();
		modal.remove();
	});
	
	modal.querySelector('.modal-close').addEventListener('click', () => {
		modal.remove();
	});
}

async function promptCreateWorkspace() {
	const name = prompt('Workspace name:');
	if (!name) return;
	
	const description = prompt('Description (optional):') || '';
	await createWorkspace(name, description);
}
