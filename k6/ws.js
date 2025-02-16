import ws from 'k6/ws';
import { check, sleep } from 'k6';
import compair from 'k6/x/async-timer'
import { randomIntBetween, randomArrayItem, getUserTopics, randomUser, requireEnv} from './util.js'
import { createNewMessage } from './api-util.js'
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.4/index.js';

const sessionDuration = randomIntBetween(20000, 20001); // user session between 50s and 55s

const wsUrl = `ws://${requireEnv('WS_HOST')}/ws`;
const { userId, clientId, cookies } = randomUser()
const authorizedTopics = getUserTopics(userId)

export const options = {
  vus: 2000, 
  iterations: 2000
};

export function setup(){}

export default function () {
	let sendMode = true
  const res = ws.connect(wsUrl, { headers: { cookie: cookies.toString() } }, function (socket) {

    socket.on('open', function open() {

      console.debug(`VU ${__VU}: connected`);
      sleep(Math.random())

      if (Math.random() < 0.2)
        socket.setInterval(function timeout() {
		      if (!sendMode) return

          const topic = randomArrayItem(authorizedTopics)
          const newMsg = compair.start() // creates new unique str
          const hResp = createNewMessage(topic, newMsg, cookies.toString())
          
          if (hResp.status === 201) {
            compair.sent(newMsg)
          } else 
            console.error(hResp)

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
      compair.end(obj.text)
    })

    socket.on('close', function (code) {
      check(code, {'wsCloseCode': c => c === 1000 || c === 1001})

      console.debug(`VU ${__VU}: disconnected`);
    });

    socket.setTimeout(function (){
      sendMode = false;
    }, sessionDuration)

    socket.setTimeout(function () {
      socket.close(1001); // going away
    }, sessionDuration + 19000);
  });

  check(res, { 'Connected successfully': (r) => r && r.status === 101 });
	if (res.status !== 101){
		console.error('websocket not connected', {status: res.status, body: res.body, res: res})
	}
}

export function handleSummary(data) {
  const outputFile = `summary/ws_${(new Date()).toISOString()}.json`
  console.log('this test exported to file ', outputFile)
  return {
    stdout: textSummary(data),
    [outputFile]: JSON.stringify(data),
  };
}
