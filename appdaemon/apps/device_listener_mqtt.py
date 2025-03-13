import hassapi as hass
import json

class DeviceLister(hass.Hass):
    def initialize(self):
        self.log("Starting Device Lister (MQTT Version)")
        
        # Subscribe to all Zigbee2MQTT topics
        self.mqtt_subscribe("zigbee2mqtt/#", namespace="mqtt")
        self.listen_event(self.mqtt_callback, "MQTT_MESSAGE", namespace="mqtt")
        
        # Log that we're listening
        self.log("Listening for MQTT messages...")
    
    def mqtt_callback(self, event_name, data, kwargs):
        """Handle incoming MQTT messages"""
        topic = data.get('topic', '')
        payload_str = data.get('payload', '{}')
        
        try:
            payload = json.loads(payload_str)
        except json.JSONDecodeError:
            self.log(f"Failed to parse JSON payload: {payload_str}")
            return
            
        # Only process messages from button devices
        if 'botao' in topic:
            self.log("=== Button MQTT Message ===")
            self.log(f"Topic: {topic}")
            self.log(f"Payload: {payload}")
            
            # If this is an action message
            if isinstance(payload, dict) and 'action' in payload:
                self.log(f"Action detected: {payload['action']}")
            
            self.log("---") 