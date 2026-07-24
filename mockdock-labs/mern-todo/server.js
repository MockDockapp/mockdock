const express = require('express');
const mongoose = require('mongoose');

const app = express();
app.use(express.json());

const mongoUri = process.env.MONGO_URI || 'mongodb://localhost:27017/todos';
const port = process.env.PORT || 3000;

// Connect to Database
// EDUCATIONAL NOTE: In Real Mode, this connects directly to the mern-mongodb container.
// In Ghost Mode, MockDock redirects connection calls to the MockDock proxy daemon
// which serves as an in-memory SQL/NoSQL mock engine.
mongoose.connect(mongoUri)
  .then(() => console.log('✅ Connected to Database'))
  .catch(err => console.error('❌ Database connection error:', err));

// Todo Schema definition
const TodoSchema = new mongoose.Schema({
  title: { type: String, required: true },
  completed: { type: Boolean, default: false }
});
const Todo = mongoose.model('Todo', TodoSchema);

// HTML Frontend serving
app.get('/', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head>
      <title>MockDock MERN To Do Sandbox</title>
      <style>
        body { font-family: sans-serif; background: #0f172a; color: #f1f5f9; padding: 40px; display: flex; justify-content: center; }
        .card { background: #1e293b; padding: 24px; border-radius: 12px; box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1); width: 400px; }
        input[type="text"] { width: 75%; padding: 8px; border-radius: 6px; border: 1px solid #475569; background: #334155; color: #fff; }
        button { padding: 8px 12px; border: none; border-radius: 6px; background: #3b82f6; color: #fff; cursor: pointer; }
        ul { list-style: none; padding: 0; }
        li { display: flex; justify-content: space-between; padding: 8px; background: #334155; margin-bottom: 8px; border-radius: 6px; }
      </style>
    </head>
    <body>
      <div class="card">
        <h2>MockDock MERN To Do</h2>
        <div style="display: flex; gap: 8px;">
          <input type="text" id="todo-input" placeholder="Enter new todo...">
          <button onclick="addTodo()">Add</button>
        </div>
        <ul id="todo-list"></ul>
      </div>
      <script>
        async function fetchTodos() {
          const res = await fetch('/api/todos');
          const todos = await res.json();
          const list = document.getElementById('todo-list');
          list.innerHTML = '';
          todos.forEach(t => {
            list.innerHTML += \`<li>\${t.title} <button onclick="deleteTodo('\${t._id}')">Delete</button></li>\`;
          });
        }
        async function addTodo() {
          const input = document.getElementById('todo-input');
          await fetch('/api/todos', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ title: input.value })
          });
          input.value = '';
          fetchTodos();
        }
        async function deleteTodo(id) {
          await fetch('/api/todos/' + id, { method: 'DELETE' });
          fetchTodos();
        }
        fetchTodos();
      </script>
    </body>
    </html>
  `);
});

// REST API Endpoints
// EDUCATIONAL NOTE: In Ghost Mode, you can mock these endpoints on MockDock's side panel
// using stateful JSON CRUD collections without having a real MongoDB service active at all!

app.get('/api/todos', async (req, res) => {
  try {
    const todos = await Todo.find();
    res.json(todos);
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.post('/api/todos', async (req, res) => {
  try {
    const todo = new Todo({ title: req.body.title });
    await todo.save();
    res.status(201).json(todo);
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.delete('/api/todos/:id', async (req, res) => {
  try {
    await Todo.findByIdAndDelete(req.params.id);
    res.json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.listen(port, () => {
  console.log(`🚀 MERN To Do Sandbox active on http://localhost:${port}`);
});
