const API_BASE = 'http://localhost:8080/api';

class TodoApp {
    constructor() {
        this.todos = [];
        this.init();
    }
    
    async init() {
        // Setup event listeners
        document.getElementById('addButton').addEventListener('click', () => this.addTodo());
        document.getElementById('todoInput').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.addTodo();
        });
        
        // Load initial todos
        await this.loadTodos();
    }
    
    async loadTodos() {
        try {
            const response = await fetch(`${API_BASE}/todos`);
            this.todos = await response.json();
            this.render();
        } catch (error) {
            console.error('Failed to load todos:', error);
        }
    }
    
    async addTodo() {
        const input = document.getElementById('todoInput');
        const title = input.value.trim();
        
        if (!title) return;
        
        try {
            const response = await fetch(`${API_BASE}/todos/create`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ title })
            });
            
            const todo = await response.json();
            this.todos.push(todo);
            this.render();
            input.value = '';
        } catch (error) {
            console.error('Failed to add todo:', error);
        }
    }
    
    async toggleTodo(id, completed) {
        try {
            const response = await fetch(`${API_BASE}/todos/update`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id, completed })
            });
            
            const updatedTodo = await response.json();
            const index = this.todos.findIndex(t => t.id === id);
            if (index !== -1) {
                this.todos[index] = updatedTodo;
                this.render();
            }
        } catch (error) {
            console.error('Failed to update todo:', error);
        }
    }
    
    render() {
        const container = document.getElementById('todoList');
        
        if (this.todos.length === 0) {
            container.innerHTML = '<p class="empty-state">No todos yet. Add one above!</p>';
            return;
        }
        
        container.innerHTML = this.todos
            .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
            .map(todo => `
                <div class="todo-item ${todo.completed ? 'completed' : ''}">
                    <input 
                        type="checkbox" 
                        ${todo.completed ? 'checked' : ''}
                        onchange="app.toggleTodo('${todo.id}', ${!todo.completed})"
                    >
                    <span class="todo-title">${this.escapeHtml(todo.title)}</span>
                    <span class="todo-time">${this.formatTime(todo.created_at)}</span>
                </div>
            `).join('');
    }
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    formatTime(dateString) {
        const date = new Date(dateString);
        const now = new Date();
        const diff = now - date;
        
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return date.toLocaleDateString();
    }
}

// Initialize app when DOM is ready
const app = new TodoApp();