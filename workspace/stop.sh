#!/bin/bash

srsepc_pid=$(ps -aux | grep -v 'grep'  | grep srsepc | awk '{print $2}')
srsenb_pid=$(ps -aux | grep -v 'grep'  | grep srsenb | awk '{print $2}')
echo $srsenb_pid
echo $srsepc_pid
if [ ! -n "$srsenb_pid" ] 
then
    echo "srsenb have been killed......"
else
    echo "srsenb pid: $srsenb_pid are stopping......"
    kill $srsenb_pid
    echo "srsenb close complete......"
fi

if [ ! -n "$srsepc_pid" ] 
then
    echo "srsepc have been killed......"
else
    echo "srsepc pid: $srsepc_pid are stopping......"
    kill $srsepc_pid
    echo " srsepc close complete......"
fi