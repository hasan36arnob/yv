function addTodo() {
    const input = document.getElementById('todoInput');
    const todoList = document.getElementById('todoList');

    if (input.value.trim() === '') {
        alert('Please enter a task!');
        return;
    }

    const li = document.createElement('li');
    const taskText = document.createTextNode(input.value);

    const completeButton = document.createElement('button');
    completeButton.innerHTML = 'Complete';
    completeButton.onclick = function() {
        li.classList.toggle('completed');
    };

    const deleteButton = document.createElement('button');
    deleteButton.innerHTML = 'Delete';
    deleteButton.onclick = function() {
        todoList.removeChild(li);
    };

    li.appendChild(taskText);
    li.appendChild(completeButton);
    li.appendChild(deleteButton);

    todoList.appendChild(li);
    input.value = '';
}
