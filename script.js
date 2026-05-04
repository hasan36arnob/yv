const todoInput = document.getElementById('todoInput');
const addBtn = document.getElementById('addBtn');
const todoList = document.getElementById('todoList');
const taskCount = document.getElementById('task-count');
const completedCount = document.getElementById('completed-count');
const filterAll = document.getElementById('filter-all');
const filterActive = document.getElementById('filter-active');
const filterCompleted = document.getElementById('filter-completed');
const clearCompleted = document.getElementById('clear-completed');
const pricingToggle = document.getElementById('pricing-toggle');
const mobileMenuBtn = document.getElementById('mobile-menu-btn');
const navbar = document.querySelector('.navbar');
const contactForm = document.getElementById('contactForm');
const contactMessage = document.getElementById('contactMessage');

const API_URL = '/api';
let tasks = [];
let activeFilter = 'all';
let useAPI = false;

const pricingPlans = [
  { selector: '.pricing-card:nth-child(1) .amount', monthly: 4, yearly: 40 },
  { selector: '.pricing-card:nth-child(2) .amount', monthly: 9, yearly: 90 },
  { selector: '.pricing-card:nth-child(3) .amount', monthly: 15, yearly: 150 }
];

const checkAPI = async () => {
  try {
    const response = await fetch(`${API_URL.replace('/api', '')}/health`);
    if (response.ok) {
      useAPI = true;
      console.log('Connected to backend API');
      return true;
    }
  } catch (error) {
    console.log('Backend API not available. Using localStorage fallback.');
  }
  useAPI = false;
  tasks = JSON.parse(localStorage.getItem('todoProTasks')) || [];
  return false;
};

const saveTasks = async () => {
  if (!useAPI) {
    localStorage.setItem('todoProTasks', JSON.stringify(tasks));
  }
};

const fetchTasksFromAPI = async () => {
  try {
    const response = await fetch(`${API_URL}/tasks`);
    if (!response.ok) throw new Error('Failed to fetch tasks');
    tasks = await response.json() || [];
    renderTasks();
    updateCounts();
  } catch (error) {
    console.error('Error fetching tasks:', error);
  }
};

const createTaskViaAPI = async (text) => {
  try {
    const response = await fetch(`${API_URL}/tasks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text, completed: false })
    });
    if (!response.ok) throw new Error('Failed to create task');
    const newTask = await response.json();
    tasks.unshift(newTask);
    renderTasks();
    updateCounts();
  } catch (error) {
    console.error('Error creating task:', error);
    alert('Failed to create task. Please try again.');
  }
};

const updateTaskViaAPI = async (id, updates) => {
  try {
    const response = await fetch(`${API_URL}/tasks/update?id=${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(updates)
    });
    if (!response.ok) throw new Error('Failed to update task');
    const index = tasks.findIndex(t => t.id === id);
    if (index !== -1) {
      tasks[index] = { ...tasks[index], ...updates };
    }
    renderTasks();
    updateCounts();
  } catch (error) {
    console.error('Error updating task:', error);
    alert('Failed to update task. Please try again.');
  }
};

const deleteTaskViaAPI = async (id) => {
  try {
    const response = await fetch(`${API_URL}/tasks/delete?id=${id}`, {
      method: 'DELETE'
    });
    if (!response.ok) throw new Error('Failed to delete task');
    tasks = tasks.filter(t => t.id !== id);
    renderTasks();
    updateCounts();
  } catch (error) {
    console.error('Error deleting task:', error);
    alert('Failed to delete task. Please try again.');
  }
};


const renderTasks = () => {
  todoList.innerHTML = '';
  const filtered = tasks.filter(task => {
    if (activeFilter === 'active') return !task.completed;
    if (activeFilter === 'completed') return task.completed;
    return true;
  });

  if (filtered.length === 0) {
    todoList.innerHTML = '<li class="empty-state">No tasks yet. Add one to get started.</li>';
    return;
  }

  filtered.forEach(task => {
    const li = document.createElement('li');
    li.className = task.completed ? 'completed' : '';

    const label = document.createElement('span');
    label.textContent = task.text;

    const actions = document.createElement('div');
    actions.className = 'todo-actions';

    const completeButton = document.createElement('button');
    completeButton.className = 'complete';
    completeButton.textContent = task.completed ? 'Undo' : 'Complete';
    completeButton.addEventListener('click', () => toggleComplete(task.id));

    const deleteButton = document.createElement('button');
    deleteButton.className = 'delete';
    deleteButton.textContent = 'Delete';
    deleteButton.addEventListener('click', () => removeTask(task.id));

    actions.appendChild(completeButton);
    actions.appendChild(deleteButton);
    li.appendChild(label);
    li.appendChild(actions);
    todoList.appendChild(li);
  });
};

const updateCounts = () => {
  taskCount.textContent = tasks.length;
  completedCount.textContent = tasks.filter(task => task.completed).length;
};

const addTask = () => {
  const text = todoInput.value.trim();
  if (!text) {
    todoInput.focus();
    return;
  }

  if (useAPI) {
    createTaskViaAPI(text);
  } else {
    tasks.unshift({
      id: Date.now(),
      text,
      completed: false,
      createdAt: new Date().toISOString()
    });
    saveTasks();
    renderTasks();
    updateCounts();
  }

  todoInput.value = '';
};

const toggleComplete = id => {
  const task = tasks.find(t => t.id === id);
  if (!task) return;

  if (useAPI) {
    updateTaskViaAPI(id, { ...task, completed: !task.completed });
  } else {
    task.completed = !task.completed;
    saveTasks();
    renderTasks();
    updateCounts();
  }
};

const removeTask = id => {
  if (useAPI) {
    deleteTaskViaAPI(id);
  } else {
    tasks = tasks.filter(task => task.id !== id);
    saveTasks();
    renderTasks();
    updateCounts();
  }
};

const clearCompletedTasks = () => {
  if (useAPI) {
    const completedIds = tasks.filter(t => t.completed).map(t => t.id);
    completedIds.forEach(id => deleteTaskViaAPI(id));
  } else {
    tasks = tasks.filter(task => !task.completed);
    saveTasks();
    renderTasks();
    updateCounts();
  }
};

const setActiveFilter = filter => {
  activeFilter = filter;
  [filterAll, filterActive, filterCompleted].forEach(button => button.classList.toggle('active', button.id === `filter-${filter}`));
  renderTasks();
};

const applyPricingMode = () => {
  const isYearly = pricingToggle.checked;
  pricingPlans.forEach(plan => {
    const element = document.querySelector(plan.selector);
    if (!element) return;
    element.textContent = isYearly ? plan.yearly : plan.monthly;
  });
};

const toggleMobileMenu = () => {
  const expanded = mobileMenuBtn.getAttribute('aria-expanded') === 'true';
  mobileMenuBtn.setAttribute('aria-expanded', String(!expanded));
  navbar.classList.toggle('open');
};

const submitContactForm = event => {
  event.preventDefault();
  const name = document.getElementById('name').value.trim();
  const email = document.getElementById('email').value.trim();
  const message = document.getElementById('message').value.trim();

  if (!name || !email || !message) {
    contactMessage.textContent = 'Please fill in all required fields before sending.';
    contactMessage.style.color = '#dc2626';
    return;
  }

  contactMessage.textContent = 'Thank you! Your message has been queued. We will reply within one business day.';
  contactMessage.style.color = '#166534';
  contactForm.reset();
};

addBtn.addEventListener('click', addTask);

todoInput.addEventListener('keydown', event => {
  if (event.key === 'Enter') addTask();
});

filterAll.addEventListener('click', () => setActiveFilter('all'));
filterActive.addEventListener('click', () => setActiveFilter('active'));
filterCompleted.addEventListener('click', () => setActiveFilter('completed'));
clearCompleted.addEventListener('click', clearCompletedTasks);
pricingToggle.addEventListener('change', applyPricingMode);
mobileMenuBtn.addEventListener('click', toggleMobileMenu);
contactForm.addEventListener('submit', submitContactForm);

document.addEventListener('click', event => {
  if (!navbar.contains(event.target) && !mobileMenuBtn.contains(event.target) && navbar.classList.contains('open')) {
    navbar.classList.remove('open');
    mobileMenuBtn.setAttribute('aria-expanded', 'false');
  }
});

const init = async () => {
  await checkAPI();
  if (useAPI) {
    await fetchTasksFromAPI();
  } else {
    renderTasks();
    updateCounts();
  }
  applyPricingMode();
  setActiveFilter('all');
};

init();
