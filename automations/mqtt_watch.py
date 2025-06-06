import paho.mqtt.client as mqtt
import json
import dotenv
import os

dotenv.load_dotenv()

MQTT_HOST = os.environ["MQTT_HOST"]
MQTT_PORT = int(os.environ["MQTT_PORT"])
MQTT_TOPIC = "zigbee2mqtt/0xa4c138616b4937b2"

def on_connect(client, userdata, flags, rc, properties=None):
    if rc == 0:
        print(f"Connected successfully, subscribing to {MQTT_TOPIC}")
        client.subscribe(MQTT_TOPIC)
    else:
        print(f"Connection failed with code {rc}")

def on_message(client, userdata, msg):
    try:
        data = json.loads(msg.payload.decode())
        if "target_distance" in data:
            print(f"Distance: {data['target_distance']}")
    except Exception as e:
        print(f"Error processing message: {e}")

def main():
    # Use WebSockets for external access, regular MQTT for in-cluster
    use_websockets = MQTT_PORT == 443
    
    client = mqtt.Client(protocol=mqtt.MQTTv5, transport="websockets" if use_websockets else "tcp")
    client.on_connect = on_connect
    client.on_message = on_message

    if use_websockets:
        client.ws_set_options(path="/")
        client.tls_set()
    
    print(f"Connecting to {MQTT_HOST}:{MQTT_PORT} via {'WebSocket' if use_websockets else 'MQTT'}")
    client.connect(MQTT_HOST, MQTT_PORT, 60)
    client.loop_forever()

if __name__ == "__main__":
    main() 