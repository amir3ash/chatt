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
    const id = randomIntBetween(1, 2000)
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
    const path = '/tmp/.dummy_authorized_topics.csv'
    console.info('Parsing authz data ...')

    /** @type string[][] */
    const res = new Array(2000)
    const lines = open(path).split('\n')

    for(const line of lines){
        const [userId, ...topics] = line.split(',')
        const userIdInt = parseInt(userId)
        
        if (!isNaN(userIdInt))
            res[userIdInt] = topics.filter(a=> a)
    }
    
    return res
});
  
export function getUserTopics(userId){
    const user = parseInt(userId)
    return data[user]
}
