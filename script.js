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

let tasks = JSON.parse(localStorage.getItem('todoProTasks')) || [];
let activeFilter = 'all';

const pricingPlans = [
  { selector: '.pricing-card:nth-child(1) .amount', monthly: 4, yearly: 40 },
  { selector: '.pricing-card:nth-child(2) .amount', monthly: 9, yearly: 90 },
  { selector: '.pricing-card:nth-child(3) .amount', monthly: 15, yearly: 150 }
];

const init = () => {
  renderTasks();
  updateCounts();
  applyPricingMode();
  setActiveFilter('all');
};

const saveTasks = () => {
  localStorage.setItem('todoProTasks', JSON.stringify(tasks));
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

  tasks.unshift({
    id: Date.now(),
    text,
    completed: false,
    createdAt: new Date().toISOString()
  });

  todoInput.value = '';
  saveTasks();
  renderTasks();
  updateCounts();
};

const toggleComplete = id => {
  tasks = tasks.map(task => task.id === id ? { ...task, completed: !task.completed } : task);
  saveTasks();
  renderTasks();
  updateCounts();
};

const removeTask = id => {
  tasks = tasks.filter(task => task.id !== id);
  saveTasks();
  renderTasks();
  updateCounts();
};

const clearCompletedTasks = () => {
  tasks = tasks.filter(task => !task.completed);
  saveTasks();
  renderTasks();
  updateCounts();
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

init();
