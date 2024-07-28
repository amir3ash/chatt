# Simple high performance chat/live-comment service

- Suports websocket, http
- Using Kafka for scalability
- Reducing number of goroutins for websockets by using ED design
- Websocket Server consumes new messages using CDC
- Implements access control via SpiceDB
- Saves messages to mongodb via bucket pattern
- Clean architecture
- OpenTelementry support
