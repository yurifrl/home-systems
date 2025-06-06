import paho.mqtt.client as mqtt
import json
import sys

def on_connect(client, userdata, flags, rc, properties=None):
    print(f"Connection attempt result: {rc}")
    if isinstance(rc, mqtt.connack_string):
        print(f"Connection string: {mqtt.connack_string(rc)}")
    print(f"Flags: {flags}")
    if rc == 0:
        print("Successfully connected to broker")
        print("Subscribing to zigbee2mqtt/0xa4c138616b4937b2")
        client.subscribe("zigbee2mqtt/0xa4c138616b4937b2")
    else:
        print(f"Bad connection, returned code={rc}")

def on_disconnect(client, userdata, rc, properties=None):
    print(f"Disconnected with result code: {rc}")

def on_message(client, userdata, msg):
    print(f"Received message on topic: {msg.topic}")
    try:
        data = json.loads(msg.payload.decode())
        print(f"Full message: {data}")
        if "target_distance" in data:
            print(f"Target Distance: {data['target_distance']}")
    except json.JSONDecodeError as e:
        print(f"Failed to decode JSON: {e}")
        print(f"Raw message: {msg.payload}")
    except Exception as e:
        print(f"Unexpected error: {e}")

def on_subscribe(client, userdata, mid, granted_qos, properties=None):
    print(f"Subscribed with message ID: {mid}")
    print(f"Granted QOS: {granted_qos}")

print("Starting MQTT Client...")
print(f"Using MQTT version: {mqtt.MQTTv5}")

try:
    client = mqtt.Client(protocol=mqtt.MQTTv5, transport="websockets")
    client.on_connect = on_connect
    client.on_message = on_message
    client.on_disconnect = on_disconnect
    client.on_subscribe = on_subscribe

    print("Attempting to connect to broker...")
    print("Host: mosquitto-websockets.syscd.tech")
    print("Port: 443")
    print("Transport: WebSocket")
    
    # Try with a simpler path
    client.ws_set_options(path="/")
    client.tls_set()  # Enable TLS since we're using 443
    client.connect("mosquitto-websockets.syscd.tech", 443, 60)
    print("Starting loop...")
    client.loop_forever()
except Exception as e:
    print(f"Fatal error: {e}", file=sys.stderr)
    raise 