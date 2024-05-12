# Simple high performance chat/live-comment service

- Suports websocket, http
- Reducing number of goroutins for websockets by using ED design
- Implements access control via SpiceDB
- Reduces number of requests to SpiceDB via BulkCheck
- Saves messages to mongodb via bucket pattern
- Clean architecture
