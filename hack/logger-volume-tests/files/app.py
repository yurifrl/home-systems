from flask import Flask, render_template_string
import os
from datetime import datetime
import json

app = Flask(__name__)

HTML_TEMPLATE = """
<!DOCTYPE html>
<html>
<head>
    <title>Volume Persistence Tester</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .status-box { 
            border: 1px solid #ddd; 
            padding: 20px; 
            margin: 20px 0; 
            border-radius: 5px;
        }
        .log-entry { 
            font-family: monospace; 
            margin: 5px 0;
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin: 20px 0;
        }
        .stat-item {
            background: #f5f5f5;
            padding: 15px;
            border-radius: 5px;
        }
        .refresh-button {
            padding: 10px 20px;
            background: #007bff;
            color: white;
            border: none;
            border-radius: 5px;
            cursor: pointer;
        }
        .refresh-button:hover {
            background: #0056b3;
        }
    </style>
    <script>
        function refreshPage() {
            location.reload();
        }
        
        // Auto refresh every 5 seconds
        setInterval(refreshPage, 5000);
    </script>
</head>
<body>
    <h1>Volume Persistence Tester</h1>
    
    <button onclick="refreshPage()" class="refresh-button">Refresh Now</button>
    
    <div class="stats">
        <div class="stat-item">
            <h3>Pod Info</h3>
            <p>Pod Name: {{ pod_name }}</p>
            <p>Node: {{ node_name }}</p>
        </div>
        <div class="stat-item">
            <h3>Volume Stats</h3>
            <p>Volume Age: {{ volume_age }}</p>
            <p>Write Count: {{ write_count }}</p>
        </div>
        <div class="stat-item">
            <h3>Current Time</h3>
            <p>{{ current_time }}</p>
        </div>
    </div>
    
    <div class="status-box">
        <h2>Recent Events</h2>
        {% for entry in recent_events %}
        <div class="log-entry">{{ entry }}</div>
        {% endfor %}
    </div>
    
    <div class="status-box">
        <h2>Volume History</h2>
        {% for entry in volume_history %}
        <div class="log-entry">{{ entry }}</div>
        {% endfor %}
    </div>
</body>
</html>
"""

DATA_DIR = '/data'
HISTORY_FILE = f'{DATA_DIR}/history.json'
EVENTS_FILE = f'{DATA_DIR}/events.json'
COUNTER_FILE = f'{DATA_DIR}/counter.txt'
INIT_FILE = f'{DATA_DIR}/init.txt'

def initialize_volume():
    os.makedirs(DATA_DIR, exist_ok=True)
    
    if not os.path.exists(INIT_FILE):
        with open(INIT_FILE, 'w') as f:
            f.write(datetime.now().isoformat())
    
    if not os.path.exists(COUNTER_FILE):
        with open(COUNTER_FILE, 'w') as f:
            f.write('0')
    
    if not os.path.exists(HISTORY_FILE):
        with open(HISTORY_FILE, 'w') as f:
            json.dump([], f)
    
    if not os.path.exists(EVENTS_FILE):
        with open(EVENTS_FILE, 'w') as f:
            json.dump([], f)

def increment_counter():
    with open(COUNTER_FILE, 'r') as f:
        count = int(f.read().strip() or '0')
    with open(COUNTER_FILE, 'w') as f:
        f.write(str(count + 1))
    return count + 1

def add_event(event):
    try:
        with open(EVENTS_FILE, 'r') as f:
            events = json.load(f)
    except:
        events = []
    
    events.insert(0, f"[{datetime.now().isoformat()}] {event}")
    events = events[:50]  # Keep only last 50 events
    
    with open(EVENTS_FILE, 'w') as f:
        json.dump(events, f)

@app.route('/')
def home():
    initialize_volume()
    count = increment_counter()
    
    # Add event for page view
    add_event(f"Page viewed by pod {os.environ.get('POD_NAME')}")
    
    # Get volume initialization time
    with open(INIT_FILE, 'r') as f:
        init_time = f.read().strip()
    
    # Calculate volume age
    init_datetime = datetime.fromisoformat(init_time)
    age_minutes = (datetime.now() - init_datetime).total_seconds() / 60
    
    # Read recent events
    with open(EVENTS_FILE, 'r') as f:
        recent_events = json.load(f)
    
    return render_template_string(HTML_TEMPLATE,
        pod_name=os.environ.get('POD_NAME', 'unknown'),
        node_name=os.environ.get('NODE_NAME', 'unknown'),
        volume_age=f"{age_minutes:.1f} minutes",
        write_count=count,
        current_time=datetime.now().strftime('%Y-%m-%d %H:%M:%S'),
        recent_events=recent_events[:10],
        volume_history=recent_events[10:30]
    )

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000) 