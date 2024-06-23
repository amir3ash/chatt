import ws from 'k6/ws';
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const wsMsgRecieved = new Trend('ws_waiting_time', true);
const wsErrors = new Rate('ws_errors');

//const host = 'chatting-chat-svc.default.svc.cluster.local'
const { API_HOST, WS_HOST, AUTHZED_HOST, AUTHZED_TOKEN } = __ENV

const AuthzedAuthorizationHeader = 'Bearer ' + AUTHZED_TOKEN

const sessionDuration = randomIntBetween(50000, 55000); // user session between 50s and 55s

function randomIntBetween(min, max) { // min and max included
  return Math.floor(Math.random() * (max - min + 1) + min);
}

function randomArrayItem(arr) {
  return arr[randomIntBetween(0, arr.length - 1)]
}

const usedIds = new Map()
function randomId() {
  const id = randomIntBetween(1, 999)
  let clientId = 1
  if(usedIds.has(id))
    clientId = usedIds.get(id) + 1
  
  usedIds.set(id, clientId)
 
  return {userId: '' + id, clientId: ''+ clientId}
}


function getUserTopics(userId = '343') {
  const resp = http.post(`http://${AUTHZED_HOST}:8443/v1/permissions/resources`,
    `{
        "consistency": {
            "minimizeLatency": true
        },
        "resourceObjectType": "topic",
        "permission": "writer",
        "subject": {
            "object": {
                "objectType": "user",
                "objectId": "${userId}"
            }
        }
    }`,
    {
      headers: { 'Content-Type': 'application/json', 'Authorization': AuthzedAuthorizationHeader },
    })

  if (resp.status !== 200)
    return

  /**
   * @type string[]
   */
  const topics = [] 
  for (const res of resp.body.toString().split('\n')) {
    if (!res)
      continue

    const j = JSON.parse(res)
    if (j.result && j.result.resourceObjectId)
      topics.push(j.result.resourceObjectId)
  }

  return topics
}

export const options = {
  vus: 1000, 
  iterations: 1000
};


export default function () {
  const wsUrl = `ws://${WS_HOST}/ws`;
  const {userId, clientId} = randomId()
  const authorizedTopics = getUserTopics(userId)

  let seqId = 0
  const wsTimer = new WSTimer()

  const res = ws.connect(wsUrl, { headers: { cookie: `userId=${userId}` } }, function (socket) {

    socket.on('open', function open() {

      console.log(`VU ${__VU}: connected`);
      sleep(Math.random())

      if (Math.random() < 0.1)
        socket.setInterval(function timeout() {
          const topic = randomArrayItem(authorizedTopics)
          const httpUrl = `http://${API_HOST}/topics/${topic}/messages`
          const newMsg = `${userId}-${clientId}-${seqId++}`
          const hResp = http.post(httpUrl, `{"message":"${newMsg}"}`, {
            headers: { 'Content-Type': 'application/json' },
            cookies: { userId: userId }
          })

          if (hResp.status === 201) {
            wsTimer.AddedMessage(newMsg)
          }

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
