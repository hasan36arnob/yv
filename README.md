# TodoPro - Full Stack Task Management App

A modern, production-ready todo application with a responsive frontend and Go backend using JSON-backed storage.

## 🏗️ Project Structure

```
d:\yv\
├── index.html          # Frontend landing page and todo app
├── style.css           # Responsive design and styling
├── script.js           # Frontend logic with API integration
├── main.go            # Go backend server
├── go.mod             # Go module file
├── go.sum             # Go dependency checksums
├── tasks.json         # JSON data storage file (auto-created)
├── Dockerfile         # Container build definition
├── docker-compose.yml # Multi-service container config
├── .dockerignore      # Docker ignore rules
├── README.md          # This file
└── .gitignore         # Git ignore rules
```

## 🚀 Quick Start

### Prerequisites
- **Go 1.21+** - [Download Go](https://golang.org/dl)
- **Windows PowerShell** or any terminal

### Step 1: Build and Run the Backend

```powershell
cd d:\yv
go build -o todopro.exe
.\todopro.exe
```

Output:
```
TodoPro API server running on http://localhost:5000
Endpoints:
  GET    /api/tasks         - Fetch all tasks
  POST   /api/tasks         - Create a new task
  PUT    /api/tasks/update  - Update a task (with ?id=taskId)
  DELETE /api/tasks/delete  - Delete a task (with ?id=taskId)
  GET    /health           - Health check

Storage: tasks.json
```

The backend will:
- Create `tasks.json` automatically on first run
- Listen on `http://localhost:5000`
- Enable CORS for frontend communication
- Provide a RESTful API for task management

### Step 2: Run the Frontend

Open another PowerShell window in the same directory and run a local server:

```powershell
cd d:\yv
python -m http.server 8000
```

Or using Node.js:
```powershell
npx http-server -p 8000
```

Then open your browser to: **http://localhost:8000**

## 🔗 API Endpoints

### Get All Tasks
```bash
GET http://localhost:5000/api/tasks
```

### Create Task
```bash
POST http://localhost:5000/api/tasks
Content-Type: application/json

{
  "text": "My new task",
  "completed": false
}
```

### Update Task
```bash
PUT http://localhost:5000/api/tasks/update?id=1
Content-Type: application/json

{
  "text": "Updated task text",
  "completed": true
}
```

### Delete Task
```bash
DELETE http://localhost:5000/api/tasks/delete?id=1
```

### Health Check
```bash
GET http://localhost:5000/health
```

## 💾 Data Storage

The app now uses a simple JSON-backed store for persistence. The file `tasks.json` is created automatically on first run and lives next to the backend binary.

This keeps the app lightweight and container-friendly while still preserving tasks across restarts.

## 🔄 Frontend & Backend Connection

The frontend (`script.js`) automatically:
1. Checks if the backend API is running on startup
2. Uses API calls if backend is available
3. Falls back to localStorage if backend is unavailable
4. Syncs all CRUD operations (Create, Read, Update, Delete) with the backend

**Fallback behavior:**
- Tasks work offline using browser localStorage
- When backend comes online, it syncs seamlessly
- No data loss in offline/online transitions

## 📦 Deployment

### Build for Production
```powershell
# Windows
go build -o todopro.exe

# Linux/Mac
go build -o todopro
```

### Docker Support
The project is containerized with `Dockerfile` and `docker-compose.yml`.

Build and run locally (if Docker is available):
```bash
docker build -t todopro .
docker run -p 5000:5000 -v "$PWD/data:/data" -e DATA_FILE=/data/tasks.json todopro
```

Or with Compose:
```bash
docker compose up --build
```

The container runs the Go backend and serves the static frontend from the same origin.

### No Docker on your laptop?
If your machine is low-end and cannot run Docker locally, you can use GitHub Actions to build the container remotely.

A workflow is included at `.github/workflows/docker-build.yml`. When you push to `main`, GitHub will build the Docker image and publish it to GitHub Container Registry as `ghcr.io/<your-username>/yv:latest`.

> This means you can still use containerization without installing Docker locally.

## 🔧 Development

### Adding Go Dependencies
```powershell
go get github.com/username/package
go mod tidy
```

### Rebuild After Changes
```powershell
go build -o todopro.exe
.\todopro.exe
```

### Data Management
To reset the data store:
```powershell
Remove-Item tasks.json
.\todopro.exe  # Recreates empty JSON store
```

## 📝 Git Workflow for GitHub Green

```powershell
cd d:\yv

# Check status
git status

# Stage all changes
git add .

# Commit with descriptive message
git commit -m "Add backend API with Go and JSON storage"

# Push to main branch
git push origin main
```

**Tips for GitHub Green:**
- Make **small, focused commits** frequently
- Use meaningful commit messages
- Push regularly (daily or every few hours)
- Work on different features in separate branches

Example commit sequence:
```powershell
git commit -m "Create Go backend with CRUD operations"
git commit -m "Add JSON storage integration"
git commit -m "Implement CORS for frontend communication"
git commit -m "Update frontend to use API endpoints"
git commit -m "Add fallback to localStorage when API unavailable"
```

## 🐛 Troubleshooting

### Backend won't start
- Check port 5000 is not already in use
- Ensure Go is installed: `go version`
- Delete `tasks.json` and restart

### Frontend can't connect to API
- Check backend is running: `curl http://localhost:5000/health`
- Verify browser console for CORS errors
- Try refreshing the page after starting backend

### Go build fails
```powershell
go mod clean
go mod tidy
go build -o todopro.exe
```

### Cannot delete tasks
- Ensure backend is running
- Check browser console for errors
- Verify task ID exists

## 📚 Technology Stack

- **Frontend**: HTML5, CSS3, Vanilla JavaScript
- **Backend**: Go 1.21+
- **Data storage**: JSON file-based
- **API**: RESTful with CORS support
- **Deployment**: Standalone executable

## 📄 Features

✅ Fully responsive design
✅ Real-time task sync
✅ localStorage fallback
✅ CORS-enabled API
✅ JSON persistence
✅ Clean, modern UI
✅ Mobile-friendly
✅ Production-ready
✅ No external service dependencies

## 📞 Support

For issues or improvements:
1. Check the troubleshooting section
2. Verify backend/frontend are both running
3. Review browser console logs
4. Check server terminal output

---

**Happy coding! Keep pushing to GitHub for those green squares! 🟢**
