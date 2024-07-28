import { SharedArray } from 'k6/data';

export function requireEnv(name){
    const env = __ENV[name]
    if (!env)
        throw new Error(`Environment variable ${name} not exists`)

    return env
}

export function randomIntBetween(min, max) { // min and max included
    return Math.floor(Math.random() * (max - min + 1) + min);
}

export function randomArrayItem(arr) {
    return arr[randomIntBetween(0, arr.length - 1)]
}


const usedIds = new Map()

export function randomUser() {
    const id = randomIntBetween(1, 999)
    let clientId = 1

    if(usedIds.has(id))
        clientId = usedIds.get(id) + 1
    
    usedIds.set(id, clientId)

    const cookies = {
        userId: id+'',
        toString: function(){
            return `userId=${id}`
        }
    }
    
    return {userId: '' + id, clientId: ''+ clientId, cookies}    
}

const data = new SharedArray('authrizedTopics', function () {
    const path = '../authz/mockAuth.yaml'
    const reg = /  topic:(?<topic>.+)#writer@user:(?<userId>\d+)/
    console.info('Parsing authz data ...')

    const res = open(path).split('\n').slice(3).filter(line => line.length > 1)
        .map(line => line.match(reg))
        .map(r => ({topic:r[1], userId: parseInt(r[2])}))

    const users = new Set(res.map(d => d.userId))

    /**
     * @type string[][]
     */
    const topics = new Array(1000)
    users.forEach(userId => topics[userId] = res.filter(d => d.userId === userId).map(d => d.topic))
    
    return topics
});
  
  

export function getUserTopics(userId){
    const user = parseInt(userId)
    return data[user]
}


// /**
//  * 
//  * @param {string} userId 
//  * @returns 
//  */
// export function getUserTopics1(userId) {
//     const AuthzedAuthorizationHeader = 'Bearer ' + AUTHZED_TOKEN

//     const resp = http.post(`http://${AUTHZED_HOST}:8443/v1/permissions/resources`,
//       `{
//           "consistency": {
//               "minimizeLatency": true
//           },
//           "resourceObjectType": "topic",
//           "permission": "writer",
//           "subject": {
//               "object": {
//                   "objectType": "user",
//                   "objectId": "${userId}"
//               }
//           }
//       }`,
//       {
//         headers: { 'Content-Type': 'application/json', 'Authorization': AuthzedAuthorizationHeader },
//       })
  
//     if (resp.status !== 200)
//       return
  
//     /**
//      * @type string[]
//      */
//     const topics = [] 
//     for (const res of resp.body.toString().split('\n')) {
//       if (!res)
//         continue
  
//       const j = JSON.parse(res)
//       if (j.result && j.result.resourceObjectId)
//         topics.push(j.result.resourceObjectId)
//     }
  
//     return topics
//   }