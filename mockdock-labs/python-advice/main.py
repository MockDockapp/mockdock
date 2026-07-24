import os
import time
from fastapi import FastAPI, HTTPException
from fastapi.responses import HTMLResponse, StreamingResponse
from openai import OpenAI
from tenacity import retry, stop_after_attempt, wait_exponential

app = FastAPI(title="MockDock Python Advice Sandbox")

# Initialize OpenAI SDK
# EDUCATIONAL NOTE: Target endpoint is api.openai.com. Under MockDock Ghost Mode,
# this gets intercepted transparently at the container network boundary.
client = OpenAI(api_key=os.getenv("OPENAI_API_KEY", "sk-mockdock-openai"))

# Tenacity Retry Block
# EDUCATIONAL NOTE: This retry decorator implements exponential backoff.
# When MockDock injects network chaos (e.g. 20% drops, 500ms latency), requests may time out or fail.
# Tenacity catches the HTTP errors and retries the OpenAI request up to 3 times.
@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=2, max=10),
    reraise=True
)
def fetch_openai_completions(prompt: str):
    response = client.chat.completions.create(
        model="gpt-3.5-turbo",
        messages=[{"role": "user", "content": prompt}],
        timeout=5.0 # 5 second timeout to trigger fail block when network is extremely slow
    )
    return response.choices[0].message.content

@app.get("/", response_class=HTMLResponse)
def index():
    return """
    <!DOCTYPE html>
    <html>
    <head>
      <title>Python LLM & Chaos Sandbox</title>
      <style>
        body { font-family: sans-serif; background: #0f172a; color: #f1f5f9; padding: 40px; display: flex; justify-content: center; }
        .card { background: #1e293b; padding: 24px; border-radius: 12px; box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1); width: 500px; }
        button { padding: 10px 16px; border: none; border-radius: 6px; background: #8b5cf6; color: #fff; cursor: pointer; font-size: 1rem; width: 100%; margin-bottom: 12px; }
        pre { background: #0f172a; padding: 12px; border-radius: 6px; font-size: 0.85rem; color: #a78bfa; min-height: 80px; white-space: pre-wrap; }
        .stat { font-size: 0.8rem; color: #94a3b8; margin-top: 8px; }
      </style>
    </head>
    <body>
      <div class="card">
        <h2>MockDock LLM & Chaos Sandbox</h2>
        <p>Uses the official <code>openai</code> python SDK with <code>tenacity</code> retry handlers.</p>
        <button onclick="getAdvice()">Get AI Coding Advice</button>
        <button onclick="getAdviceStream()" style="background: #3b82f6;">Get Streaming Response (SSE)</button>
        <h3>AI Output:</h3>
        <pre id="output">Click a button to generate advice...</pre>
        <div class="stat" id="stat"></div>
      </div>
      <script>
        async function getAdvice() {
          const out = document.getElementById('output');
          const stat = document.getElementById('stat');
          out.innerText = 'Requesting advice (retries active)...';
          stat.innerText = '';
          const start = Date.now();
          try {
            const res = await fetch('/api/advice');
            const data = await res.json();
            const elapsed = ((Date.now() - start) / 1000).toFixed(2);
            if (res.status !== 200) throw new Error(data.detail || 'API Error');
            out.innerText = data.advice;
            stat.innerText = 'Request completed in ' + elapsed + 's';
          } catch(err) {
            const elapsed = ((Date.now() - start) / 1000).toFixed(2);
            out.innerText = 'Failed: ' + err.message;
            stat.innerText = 'Request failed after ' + elapsed + 's';
          }
        }

        async function getAdviceStream() {
          const out = document.getElementById('output');
          out.innerText = '';
          try {
            const response = await fetch('/api/advice/stream');
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            while (true) {
              const { value, done } = await reader.read();
              if (done) break;
              out.innerText += decoder.decode(value);
            }
          } catch (err) {
            out.innerText = 'Stream error: ' + err.message;
          }
        }
      </script>
    </body>
    </html>
    """

@app.get("/api/advice")
def advice():
    start_time = time.time()
    try:
        advice_content = fetch_openai_completions("Give me one short tip on writing clean code.")
        return {"advice": advice_content}
    except Exception as e:
        raise HTTPException(status_code=502, detail=f"LLM connection failed: {str(e)}")

@app.get("/api/advice/stream")
def advice_stream():
    # EDUCATIONAL NOTE: In Ghost Mode, MockDock intercepts streaming completions
    # and emulates Server-Sent Events (SSE) packet generation. You can configure
    # TTFT (Time to First Token) and token delays on the dashboard, observing
    # how your front-end parser stream handles progressive chunks.
    def event_generator():
        try:
            stream = client.chat.completions.create(
                model="gpt-3.5-turbo",
                messages=[{"role": "user", "content": "Write a 3-sentence coding tip."}],
                stream=True
            )
            for chunk in stream:
                content = chunk.choices[0].delta.content
                if content:
                    yield content
        except Exception as e:
            yield f"Error in stream: {str(e)}"

    return StreamingResponse(event_generator(), media_type="text/event-stream")
