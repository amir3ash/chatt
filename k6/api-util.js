import http from 'k6/http';
import {requireEnv} from './util.js'

const API_HOST = requireEnv('API_HOST')

/**
 * 
 * @param {string} topic 
 * @param {string} message 
 * @param {string} cookies 
 * @returns
 */
export function createNewMessage(topic, message, cookies){
    const httpUrl = `http://${API_HOST}/topics/${topic}/messages`

    const hResp = http.post(httpUrl, `{"message":"${message}"}`, {
        headers: { 
            'Content-Type': 'application/json',
            'Cookie': cookies
        }
    })

    return hResp
}

/**
 * 
 * @param {string} topic 
 * @param {string} cookies 
 * @returns 
 */
export function getMessages(topic, cookies, {limit}={limit: 40}){
    const url = `http://${API_HOST}:8888/topics/${topic}/messages?limit=${limit}`
    return http.get(url, {headers: {cookie: cookies}});
}