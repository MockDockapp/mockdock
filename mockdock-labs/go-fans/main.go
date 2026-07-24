package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	dbPool    *pgxpool.Pool
	redisCli  *redis.Client
	ctx       = context.Background()
	port      = "8080"
)

func main() {
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	dbURL := os.Getenv("DB_URL")
	redisAddr := os.Getenv("REDIS_URL")

	// Connect to PostgreSQL
	// EDUCATIONAL NOTE: In Real Mode, pgx dials the PostgreSQL database container go-fans-db.
	// In Ghost Mode, MockDock redirects TCP port 5432 query traffic to its database emulation engine.
	// We can choose to translate incoming calls to a mock SQLite database by enabling "SQLite Mapping" on the MockDock dashboard.
	var err error
	dbPool, err = pgxpool.Connect(ctx, dbURL)
	if err != nil {
		log.Printf("⚠️ Postgres connection failed (will retry): %v\n", err)
	} else {
		log.Println("✅ Connected to PostgreSQL")
	}

	// Connect to Redis
	// EDUCATIONAL NOTE: MockDock intercepts port 6379 for redis calls in Ghost Mode.
	redisCli = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	log.Println("✅ Initialized Redis Client connection")

	// Start HTTP Server
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/external", handleExternalCall)

	log.Printf("🚀 Go Fans Microservice listening on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Go Fans Sandbox</title>
			<style>
				body { font-family: sans-serif; background: #0f172a; color: #f1f5f9; padding: 40px; display: flex; justify-content: center; }
				.card { background: #1e293b; padding: 24px; border-radius: 12px; box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1); width: 450px; }
				button { padding: 10px 16px; border: none; border-radius: 6px; background: #10b981; color: #fff; cursor: pointer; margin-bottom: 8px; width: 100%%; }
				pre { background: #0f172a; padding: 12px; border-radius: 6px; font-size: 0.85rem; color: #34d399; overflow-x: auto; }
			</style>
		</head>
		<body>
			<div class="card">
				<h2>Go Fans Microservice Sandbox</h2>
				<p>Demonstrates Go's strict SSL/x509 validations and infrastructure stubs.</p>
				<button onclick="fetchStats()">Fetch Database & Redis Stats</button>
				<button onclick="fetchExternal()" style="background: #3b82f6;">Hit Secure External API</button>
				<h3>Output:</h3>
				<pre id="output">Click a button to query stubs...</pre>
			</div>
			<script>
				async function fetchStats() {
					const out = document.getElementById('output');
					out.innerText = 'Querying database...';
					const res = await fetch('/api/stats');
					const data = await res.json();
					out.innerText = JSON.stringify(data, null, 2);
				}
				async function fetchExternal() {
					const out = document.getElementById('output');
					out.innerText = 'Calling secure external api...';
					const res = await fetch('/api/external');
					const data = await res.json();
					out.innerText = JSON.stringify(data, null, 2);
				}
			</script>
		</body>
		</html>
	`)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Increment request count in Redis
	redisCli.Incr(ctx, "requests_count")
	val, _ := redisCli.Get(ctx, "requests_count").Result()

	// Query DB
	var dbStatus = "Connected"
	var count int
	if dbPool != nil {
		err := dbPool.QueryRow(ctx, "SELECT count(*) FROM pg_database").Scan(&count)
		if err != nil {
			dbStatus = fmt.Sprintf("Error: %v", err)
		}
	} else {
		dbStatus = "Postgres connection not established"
	}

	fmt.Fprintf(w, `{"postgres_status": "%s", "postgres_db_count": %d, "redis_request_counter": "%s"}`, dbStatus, count, val)
}

func handleExternalCall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Dynamic SSL Hijack verification endpoint
	// EDUCATIONAL NOTE: In Go, net/http validates certificates extremely strictly.
	// MockDock dynamic intercept requires the container OS trust store to be updated (update-ca-certificates).
	// Once trust store is updated, Go calls this URL cleanly even when MockDock performs dynamic SSL generation!
	client := &http.Client{
		Timeout: 5 * time.Time(time.Second),
	}
	resp, err := client.Get("https://api.github.com/users/mockdockapp")
	if err != nil {
		fmt.Fprintf(w, `{"status": "Failed", "error": "%s"}`, err.Error())
		return
	}
	defer resp.Body.Close()

	fmt.Fprintf(w, `{"status": "Success", "http_code": %d}`, resp.StatusCode)
}
