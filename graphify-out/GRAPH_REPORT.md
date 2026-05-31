# Graph Report - home-systems  (2026-05-31)

## Corpus Check
- 7 files · ~48,004 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 29 nodes · 26 edges · 6 communities detected
- Extraction: 100% EXTRACTED · 0% INFERRED · 0% AMBIGUOUS
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]
- [[_COMMUNITY_Community 2|Community 2]]
- [[_COMMUNITY_Community 3|Community 3]]
- [[_COMMUNITY_Community 4|Community 4]]
- [[_COMMUNITY_Community 6|Community 6]]

## God Nodes (most connected - your core abstractions)
1. `DeviceLister` - 4 edges
2. `DeviceLister` - 3 edges
3. `ButtonLampControl` - 3 edges
4. `main()` - 3 edges
5. `main()` - 3 edges
6. `HelloWorld` - 2 edges
7. `build_feed_url()` - 2 edges
8. `subscribe_feed()` - 2 edges
9. `build_feed_url()` - 2 edges
10. `subscribe_feed()` - 2 edges

## Surprising Connections (you probably didn't know these)
- None detected - all connections are within the same source files.

## Communities

### Community 0 - "Community 0"
Cohesion: 0.4
Nodes (2): DeviceLister, Handle incoming MQTT messages

### Community 1 - "Community 1"
Cohesion: 0.4
Nodes (1): DeviceLister

### Community 2 - "Community 2"
Cohesion: 0.5
Nodes (1): ButtonLampControl

### Community 3 - "Community 3"
Cohesion: 0.83
Nodes (3): build_feed_url(), main(), subscribe_feed()

### Community 4 - "Community 4"
Cohesion: 0.83
Nodes (3): build_feed_url(), main(), subscribe_feed()

### Community 6 - "Community 6"
Cohesion: 0.67
Nodes (1): HelloWorld

## Knowledge Gaps
- **1 isolated node(s):** `Handle incoming MQTT messages`
  These have ≤1 connection - possible missing edges or undocumented components.
- **Thin community `Community 0`** (5 nodes): `device_listener_mqtt.py`, `DeviceLister`, `.initialize()`, `.mqtt_callback()`, `Handle incoming MQTT messages`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Community 1`** (5 nodes): `device_listener.py`, `DeviceLister`, `.event_callback()`, `.initialize()`, `.state_callback()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Community 2`** (4 nodes): `button_lamp_control.py`, `ButtonLampControl`, `.button_pressed()`, `.initialize()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Community 6`** (3 nodes): `hello.py`, `HelloWorld`, `.initialize()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **What connects `Handle incoming MQTT messages` to the rest of the system?**
  _1 weakly-connected nodes found - possible documentation gaps or missing edges._