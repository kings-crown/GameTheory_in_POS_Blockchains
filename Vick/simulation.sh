#!/bin/bash

# set the host and port to connect
HOST="localhost"
PORT=8080

# set the initial balance
BALANCE=1000

# sleep time (in seconds)
SLEEP_TIME=60  # Synchronize with the Go routine
SLEEP_TIME1=1
SLEEP_TIME2=1

# number of users to simulate
NUM_USERS=10

# base cost of adding a block
BASE_COST=15

# percentage of validators overbidding
OVERBID_PERCENT=20

# create an array of users
users=($(seq 0 $((NUM_USERS-1))))

# shuffle the array
shuffled_users=($(for i in "${users[@]}"; do echo "$i"; done | gshuf))

# create a netcat connection for each user in shuffled order
for user in "${shuffled_users[@]}"; do
  (
    { 
      echo $BALANCE; 
      
      # determine if this user will overbid - now using a random overbid percent for each user
      OVERBID_PERCENT=$(( RANDOM % 100 + 1 ))
      RANDOM_PERCENT=$(( RANDOM % 100 + 1 ))
      
      if (( RANDOM_PERCENT <= OVERBID_PERCENT )); then
        OVERBID=true
      else
        OVERBID=false
      fi

      while true; do
        BPM=$(( RANDOM % 20 + 60 ))

        # Add another random check to decide if an overbidder should overbid at each block addition
        OVERBID_CHECK=$(( RANDOM % 100 + 1 ))
        if $OVERBID && (( OVERBID_CHECK <= OVERBID_PERCENT )); then
          BID=$(( BASE_COST + RANDOM % BASE_COST + 1 ))
        else
          BID=$(( RANDOM % BASE_COST + 1 ))
        fi

        echo $BID; sleep $SLEEP_TIME1; echo $BPM; sleep $SLEEP_TIME
      done
    } | nc $HOST $PORT
  ) &
done
