# TODO API Documentation

## Base URL
`http://localhost:8080/api`

## Endpoints

### GET /todos
Retrieve all todos.

**Response:**
```json
[
  {
    "id": "1234567890",
    "title": "Example todo",
    "completed": false,
    "created_at": "2023-01-01T00:00:00Z"
  }
]
```

### POST /todos/create
Create a new todo.

**Request Body:**
```json
{
  "title": "New todo item"
}
```

**Response:**
```json
{
  "id": "1234567891",
  "title": "New todo item",
  "completed": false,
  "created_at": "2023-01-01T00:00:00Z"
}
```

### PUT /todos/update
Update a todo's completion status.

**Request Body:**
```json
{
  "id": "1234567890",
  "completed": true
}
```

**Response:**
```json
{
  "id": "1234567890",
  "title": "Example todo",
  "completed": true,
  "created_at": "2023-01-01T00:00:00Z"
}
```

---
Generated on: {{TIMESTAMP}}