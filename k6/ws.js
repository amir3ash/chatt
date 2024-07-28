import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { randomIntBetween, randomArrayItem, getUserTopics, randomUser, requireEnv} from './util.js'
import { createNewMessage } from './api-util.js'

const wsMsgRecieved = new Trend('ws_waiting_time', true);
const wsErrors = new Rate('ws_errors');

const sessionDuration = randomIntBetween(50000, 55000); // user session between 50s and 55s

const wsUrl = `ws://${requireEnv('WS_HOST')}/ws`;
const { userId, clientId, cookies } = randomUser()
const authorizedTopics = getUserTopics(userId)

export const options = {
  vus: 1000, 
  iterations: 1000
};

export function setup(){
}


export default function () {

  let seqId = 0
  const wsTimer = new WSTimer()

  const res = ws.connect(wsUrl, { headers: { cookie: cookies.toString() } }, function (socket) {

    socket.on('open', function open() {

      console.log(`VU ${__VU}: connected`);
      sleep(Math.random())

      if (Math.random() < 0.1)
        socket.setInterval(function timeout() {
          const topic = randomArrayItem(authorizedTopics)
          const newMsg = `${userId}-${clientId}-${seqId++}`

          const hResp = createNewMessage(topic, newMsg, cookies.toString())
          
          if (hResp.status === 201) {
            wsTimer.AddedMessage(newMsg)
          }

          check(hResp, { 'Sending New Message': (r) => r && r.status === 201 });

        }, randomIntBetween(500, 3000)); // say something every 0.5-3 seconds

    });


    socket.on('ping', function () {

      console.log('PING!');

    });


    socket.on('pong', function () {

      console.log('PONG!');

    });

    socket.on("message", function (e) {
      const obj = JSON.parse(e)

      const sender = obj.senderId
      if (sender !== userId)
        return

      const msg = obj.text
      wsTimer.RecivedMessage(msg)
    })


    socket.on('close', function () {

      console.log(`VU ${__VU}: disconnected`);

    });

    //socket.setTimeout(function () {

    // console.log(`VU ${__VU}: ${sessionDuration}ms passed, leaving the chat`);

    //socket.send(JSON.stringify({ event: 'LEAVE' }));

    //}, sessionDuration);


    socket.setTimeout(function () {

      console.log(`Closing the socket forcefully 3s after graceful LEAVE`);

      socket.close(1001); // going away

    }, sessionDuration + 3000);

  });


  check(res, { 'Connected successfully': (r) => r && r.status === 101 });
	if (res.status !== 101){
		console.error('websocket not connected', {status: res.status, body: res.body})
	}

}


class WSTimer {
  constructor() {
    this.map = new Map()
  }

  AddedMessage(msg_id) {
    this.map.set(msg_id, (new Date()).getTime())
  }

  RecivedMessage(msg_id) {
    const hasMsg = this.map.has(msg_id)
    wsErrors.add(!hasMsg)

    if (hasMsg) {
      const dur = (new Date()).getTime() - this.map.get(msg_id)
      wsMsgRecieved.add(dur)
      this.map.delete(msg_id)
    }
  }

}
