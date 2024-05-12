#!/bin/bash
host='10.43.93.213:8888'
host=$1
topicId='456'

for i in {1..200}; do
	random=`head -c 300 /dev/urandom | base64 | tr -d '\n' | head -c 295 `
	# echo random = $random $i

	curl --request POST \
  --url http://${host}/topics/${topicId}/messages \
  --header 'Accept: application/json, application/problem+json' \
  --header 'Content-Type: application/json' \
  --data "{\"message\":\"m${i}-${random}\"}" --max-time 3
done

