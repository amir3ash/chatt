import { sleep, check } from 'k6';
import { randomUser, randomArrayItem, getUserTopics, randomIntBetween} from './util.js'
import { getMessages } from './api-util.js'


export const options = {
  // A number specifying the number of VUs to run concurrently.
  //vus: 90,
  // A string specifying the total duration of the test run.
  //duration: '30s',
  // Key configurations for breakpoint in this section
  executor: 'ramping-arrival-rate', //Assure load increase if the system slows
  stages: [
    { duration: '3s', target: 60 }, // just slowly ramp-up to a HUGE load
    { duration: '10s', target: 1000 },
	  { duration: '20s', target: 1000 },
    { duration: '3s', target: 20 },
  ],
  
};


const { userId, cookies } = randomUser()
const authorizedTopics = getUserTopics(userId)

export function setup(){
  return {}
}


export default function() {
  const topic = randomArrayItem(authorizedTopics)

  const res = getMessages(topic, cookies.toString())
  check(res, { 'Get Messages': (r) => r && r.status === 200 });
  if (res.status !== 200){
    console.error('can not list messages', {body: res.body, status:res.status, url: res.request.url})
  }

  sleep(0.5);
}
