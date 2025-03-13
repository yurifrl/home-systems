import hassapi as hass

class ButtonLampControl(hass.Hass):
    def initialize(self):
        # Get entity ID from the config
        self.lamp = self.args["lamp_entity"]
        self.log(f"Initializing ButtonLampControl with lamp: {self.lamp}")
        
        # Listen for button state changes on both battery sensors
        self.listen_state(self.button_pressed, "sensor.botao001_battery", attribute="action")
        self.listen_state(self.button_pressed, "sensor.botao003_battery", attribute="action")
        self.log("Listening for button presses on botao001 and botao003")
    
    def button_pressed(self, entity, attribute, old, new, kwargs):
        self.log(f"Button state change detected:")
        self.log(f"  Entity: {entity}")
        self.log(f"  Attribute: {attribute}")
        self.log(f"  Old value: {old}")
        self.log(f"  New value: {new}")
        
         # Get current lamp state
        current_state = self.get_state(self.lamp)
        self.log(f"Current lamp state: {current_state}")
        
        if current_state == "off":
            # Turn lamp on if it's off
            self.log(f"Button pressed - turning {self.lamp} on")
            self.turn_on(self.lamp)
        else:
            # Turn lamp off if it's on
            self.log(f"Button pressed - turning {self.lamp} off")
            self.turn_off(self.lamp) 