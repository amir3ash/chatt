import ws from 'k6/ws';
import http, { cookieJar } from 'k6/http';
import { check } from 'k6';

//const host = 'chatting-chat-svc.default.svc.cluster.local'
const apiHost = __ENV.API_HOST
const wsHost = __ENV.WS_HOST

const sessionDuration = randomIntBetween(50000, 55000); // user session between 50s and 55s

function randomIntBetween(min, max) { // min and max included
  return Math.floor(Math.random() * (max - min + 1) + min);
}

export const options = {
  vus: 2000, 
  iterations: 2000
};


export default function () {
  const wsUrl = `ws://${wsHost}/ws`;
  const userId = '' + randomIntBetween(1, 2000)
  let seqId = 0

  const params = { tags: { my_tag: 'my ws session'}};

  const res = ws.connect(wsUrl, {headers: {cookie: `userId=${userId}`}}, function (socket) {

    socket.on('open', function open() {

      console.log(`VU ${__VU}: connected`);

      if (Math.random() < 0.2)
        socket.setInterval(function timeout() {
          const httpUrl = `http://${apiHost}/topics/t_${randomIntBetween(0, 40)}/messages`

          const hResp = http.post( httpUrl, `{"message":"m"}`, {
            headers: { 'Content-Type': 'application/json' },
          })  
      
          check(hResp, { 'Sending New Message': (r) => r && r.status === 201 });

          // socket.send(JSON.stringify({ wsID: seqId++, topicID: `t_${randomIntBetween(0, 40)}`, message: 'a'}));


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
