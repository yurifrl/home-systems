import paho.mqtt.client as mqtt
import json
import dotenv
import os

dotenv.load_dotenv()

def on_connect(client, userdata, flags, rc, properties=None):
    if rc == 0:
        client.subscribe("zigbee2mqtt/0xa4c138616b4937b2")

def on_message(client, userdata, msg):
    try:
        data = json.loads(msg.payload.decode())
        if "target_distance" in data:
            print(f"Distance: {data['target_distance']}")
    except:
        pass

client = mqtt.Client(protocol=mqtt.MQTTv5, transport="websockets")
client.on_connect = on_connect
client.on_message = on_message

client.ws_set_options(path="/")
client.tls_set()
client.connect(os.getenv("MQTT_HOST"), int(os.getenv("MQTT_PORT")), 60)
client.loop_forever() 