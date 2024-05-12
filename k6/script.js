import http from 'k6/http';
import { sleep } from 'k6';

const host = __ENV.HOST

export const options = {
  // A number specifying the number of VUs to run concurrently.
  //vus: 90,
  // A string specifying the total duration of the test run.
  //duration: '30s',
  // Key configurations for breakpoint in this section
  executor: 'ramping-arrival-rate', //Assure load increase if the system slows
  stages: [
    { duration: '30s', target: 60 }, // just slowly ramp-up to a HUGE load
    { duration: '60s', target: 1000 },
    { duration: '10s', target: 20 },
  ],
  
};

// The function that defines VU logic.
//
// See https://grafana.com/docs/k6/latest/examples/get-started-with-k6/ to learn more
// about authoring k6 scripts.
//
export default function() {
  http.get(`http://${host}:8888/topics/456/messages?limit=100`);
  sleep(1);
}
