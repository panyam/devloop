import React, { useState, useEffect } from 'react';
import axios from 'axios';
import './App.css';

const API_URL = process.env.REACT_APP_API_URL || 'http://localhost:8080/api';

function App() {
  const [tasks, setTasks] = useState([]);
  const [newTask, setNewTask] = useState({ title: '', description: '' });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    fetchTasks();
  }, []);

  const fetchTasks = async () => {
    try {
      setLoading(true);
      const response = await axios.get(`${API_URL}/tasks`);
      setTasks(response.data.tasks || []);
      setError('');
    } catch (err) {
      setError('Failed to fetch tasks');
      console.error('Error fetching tasks:', err);
    } finally {
      setLoading(false);
    }
  };

  const createTask = async (e) => {
    e.preventDefault();
    if (!newTask.title.trim()) return;

    try {
      const response = await axios.post(`${API_URL}/tasks`, newTask);
      setTasks([...tasks, response.data.task]);
      setNewTask({ title: '', description: '' });
      setError('');
    } catch (err) {
      setError('Failed to create task');
      console.error('Error creating task:', err);
    }
  };

  const toggleTask = async (taskId, completed) => {
    try {
      const response = await axios.put(`${API_URL}/tasks/${taskId}`, {
        completed: !completed
      });
      
      setTasks(tasks.map(task => 
        task.id === taskId ? response.data.task : task
      ));
      setError('');
    } catch (err) {
      setError('Failed to update task');
      console.error('Error updating task:', err);
    }
  };

  const deleteTask = async (taskId) => {
    try {
      await axios.delete(`${API_URL}/tasks/${taskId}`);
      setTasks(tasks.filter(task => task.id !== taskId));
      setError('');
    } catch (err) {
      setError('Failed to delete task');
      console.error('Error deleting task:', err);
    }
  };

  if (loading) {
    return (
      <div className="App">
        <div className="loading">Loading tasks...</div>
      </div>
    );
  }

  return (
    <div className="App">
      <header className="App-header">
        <h1>Docker Compose Task Manager</h1>
        <p>Multi-container development with devloop</p>
      </header>

      <main className="main-content">
        {error && <div className="error">{error}</div>}

        <section className="add-task">
          <h2>Add New Task</h2>
          <form onSubmit={createTask}>
            <input
              type="text"
              placeholder="Task title"
              value={newTask.title}
              onChange={(e) => setNewTask({...newTask, title: e.target.value})}
              required
            />
            <textarea
              placeholder="Task description (optional)"
              value={newTask.description}
              onChange={(e) => setNewTask({...newTask, description: e.target.value})}
              rows={3}
            />
            <button type="submit">Add Task</button>
          </form>
        </section>

        <section className="tasks">
          <h2>Tasks ({tasks.length})</h2>
          {tasks.length === 0 ? (
            <p className="no-tasks">No tasks yet. Add one above!</p>
          ) : (
            <div className="task-list">
              {tasks.map(task => (
                <div key={task.id} className={`task ${task.completed ? 'completed' : ''}`}>
                  <div className="task-content">
                    <h3>{task.title}</h3>
                    {task.description && <p>{task.description}</p>}
                    <small>Created: {new Date(task.created_at).toLocaleString()}</small>
                  </div>
                  <div className="task-actions">
                    <button
                      onClick={() => toggleTask(task.id, task.completed)}
                      className={task.completed ? 'uncomplete' : 'complete'}
                    >
                      {task.completed ? 'Mark Incomplete' : 'Mark Complete'}
                    </button>
                    <button
                      onClick={() => deleteTask(task.id)}
                      className="delete"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </main>

      <footer>
        <p>Powered by Docker Compose + devloop</p>
      </footer>
    </div>
  );
}

export default App;