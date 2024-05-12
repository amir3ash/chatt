import ws from 'k6/ws';
import http from 'k6/http';
import { check } from 'k6';

//const host = 'chatting-chat-svc.default.svc.cluster.local'
const host = __ENV.HOST

const sessionDuration = randomIntBetween(50_000, 120_000); // user session between 50s and 2m

function randomIntBetween(min, max) { // min and max included
  return Math.floor(Math.random() * (max - min + 1) + min);
}

export const options = {
  vus: 500, 
  iterations: 500
};


export default function () {
  const wsUrl = `ws://${host}:7100/ws`;
  const params = { tags: { my_tag: 'my ws session' } };

  const res = ws.connect(wsUrl, params, function (socket) {

    socket.on('open', function open() {

      console.log(`VU ${__VU}: connected`);

      if (Math.random() < 0.5)
        socket.setInterval(function timeout() {
          const hResp = http.post(`http://${host}:8888/topics/456/messages`, `{"message":"m"}`, {
            headers: { 'Content-Type': 'application/json' },
          })  
      
          check(hResp, { 'Sending New Message': (r) => r && r.status === 201 });

        }, randomIntBetween(500, 3000)); // say something every 0.5-3 seconds

    });


    socket.on('ping', function () {

      console.log('PING!');

    });


    socket.on('pong', function () {

      console.log('PONG!');

    });


    socket.on('close', function () {

      console.log(`VU ${__VU}: disconnected`);

    });

    socket.setTimeout(function () {

      console.log(`VU ${__VU}: ${sessionDuration}ms passed, leaving the chat`);

      socket.send(JSON.stringify({ event: 'LEAVE' }));

    }, sessionDuration);


    socket.setTimeout(function () {

      console.log(`Closing the socket forcefully 3s after graceful LEAVE`);

      socket.close(1001); // going away

    }, sessionDuration + 3000);

  });


  check(res, { 'Connected successfully': (r) => r && r.status === 101 });

}
