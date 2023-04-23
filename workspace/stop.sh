#!/bin/bash

srsepc_pid=$(ps -aux | grep -v 'grep'  | grep srsepc | awk '{print $2}')
srsenb_pid=$(ps -aux | grep -v 'grep'  | grep srsenb | awk '{print $2}')
tcpdump_pid=$(ps -aux | grep -v 'grep'  | grep tcpdump | awk '{print $2}')
echo $srsenb_pid
echo $srsepc_pid
echo $tcpdump_pid
if [ ! -n "$tcpdump_pid" ] 
then
    echo "tcpdump has been killed......"
else
    echo "tcpdump pid: $tcpdump_pid are stopping......"
    kill $tcpdump_pid
    echo "tcpdump close complete......"
fi
if [ ! -n "$srsenb_pid" ] 
then
    echo "srsenb has been killed......"
else
    echo "srsenb pid: $srsenb_pid are stopping......"
    kill $srsenb_pid
    echo "srsenb close complete......"
fi

if [ ! -n "$srsepc_pid" ] 
then
    echo "srsepc has been killed......"
else
    echo "srsepc pid: $srsepc_pid are stopping......"
    kill $srsepc_pid
    echo " srsepc close complete......"
fi