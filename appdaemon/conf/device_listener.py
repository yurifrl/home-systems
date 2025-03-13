
import hassapi as hass

class DeviceLister(hass.Hass):
    def initialize(self):
        self.log("Starting Device Lister")
        
        # List all entities
        entities = self.get_state()
        
        self.log("=== Button Devices ===")
        for entity_id, state in entities.items():
            # Focus on botao (button) entities
            if "botao" in entity_id:
                attributes = state.get("attributes", {})
                self.log(f"\nEntity: {entity_id}")
                self.log(f"  State: {state.get('state')}")
                self.log(f"  All Attributes: {attributes}")
        
        # Listen for all state changes and events
        self.listen_state(self.state_callback)
        self.listen_event(self.event_callback, "zigbee2mqtt_action")  # Listen for Zigbee2MQTT events
        self.listen_event(self.event_callback, "state_changed")       # Listen for state changes
        
        # Log that we're listening
        self.log("Listening for events and state changes...")
    
    def state_callback(self, entity, attribute, old, new, kwargs):
        if "botao" in entity:
            self.log("=== Button State Change ===")
            self.log(f"Entity: {entity}")
            self.log(f"Attribute: {attribute}")
            self.log(f"Old State: {old}")
            self.log(f"New State: {new}")
            self.log("---")
    
    def event_callback(self, event_name, data, kwargs):
        # Log all events to help debug
        self.log("=== Event ===")
        self.log(f"Event Name: {event_name}")
        self.log(f"Event Data: {data}")
        self.log("---") 