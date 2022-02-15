#!/bin/bash

### input
band=$1
apn=$2
mcc=$3
mnc=$4
network=$5

# setting default dl_earfcn, DL frequence, ul frequence.
dl_earfcn=3350
DL=2680
UL=2560
### band, apn, mcc, mnc, network can not be empty.
if [[ ! -n $band ]] || [[ ! -n $apn ]] || [[ ! -n $mcc ]] || [[ ! -n $mnc ]] || [[ ! -n $network ]]
then
    echo -e "\033[31mPlease run bash with band, apn, mcc, mnc & network......\033[0m"
    exit
fi

### setting dl_earfcn, DL, UL according to $band .s
case $band in
    1) dl_earfcn=300 DL=2140 UL=1950 ;;
    3) dl_earfcn=1575 DL=1842.5 UL=1747.5 ;;
    5) dl_earfcn=2525 DL=881.5 UL=836.5 ;;
    7) dl_earfcn=3350 DL=2680 UL=2560 ;;
    8) dl_earfcn=3625 DL=942.5 UL=897.5 ;;
    34) dl_earfcn=36275 DL=2017.5 UL=0.0 ;;
    39) dl_earfcn=38450 DL=1900 UL=0.0 ;;
    40) dl_earfcn=39150 DL=2350 UL=0.0 ;;
    41) dl_earfcn=40620 DL=2593 UL=0.0;;
esac
# band=0
# apn=skygoapn
# mcc=001
# mnc=01
### get the conf path
current_path=$(pwd)
## config path
epc_conf=($current_path/conf/epc_run.conf)
enb_conf=($current_path/conf/enb_run.conf)
user_db=($current_path/conf/user_db.csv)
drb_conf=($current_path/conf/drb.conf)
mbms_conf=($current_path/conf/mbms.conf)
rr_conf=($current_path/conf/rr.conf)
sib_conf=($current_path/conf/sib.conf)
## pcap path
epc_pcap=($current_path/log/srsLTE_epc.pcap)
enb_pcap=($current_path/log/srsLTE_enb.pcap)
s1ap_pcap=($current_path/log/srsLTE_enb_s1ap.pcap)
## log path
epc_run_log=($current_path/log/epc_run.log)
enb_run_log=($current_path/log/enb_run.log)
epc_log=($current_path/log/srsLTE_epc.log)
enb_log=($current_path/log/srsLTE_enb.log)

### create epc_run.conf
echo "[mme]" > $epc_conf
echo "mme_code = 0x1a" >> $epc_conf
echo "mme_group = 0x0001" >> $epc_conf
echo "tac = 0x0007" >> $epc_conf
echo "mcc = $mcc" >> $epc_conf
echo "mnc = $mnc" >> $epc_conf
echo "mme_bind_addr = 127.0.1.100" >> $epc_conf
echo "apn = $apn" >> $epc_conf
echo "dns_addr = 8.8.8.8" >> $epc_conf
echo "paging_timer = 2" >> $epc_conf
echo "" >> $epc_conf
echo "[hss]" >> $epc_conf
echo "db_file = $user_db" >> $epc_conf
echo "" >> $epc_conf
echo "[spgw]" >> $epc_conf
echo "gtpu_bind_addr   = 127.0.1.100" >> $epc_conf
echo "sgi_if_addr      = 172.16.0.1" >> $epc_conf
echo "sgi_if_name      = srs_spgw_sgi" >> $epc_conf
echo "max_paging_queue = 100" >> $epc_conf
echo "" >> $epc_conf
echo "[pcap]" >> $epc_conf
echo "enable   = true" >> $epc_conf
echo "filename = $epc_pcap" >> $epc_conf
echo "" >> $epc_conf
echo "[log]" >> $epc_conf
echo "all_level = info" >> $epc_conf
echo "all_hex_limit = 32" >> $epc_conf
echo "filename = $epc_log" >> $epc_conf

### create enb_run.conf
echo "[enb]" > $enb_conf
echo "mcc = $mcc" >> $enb_conf
echo "mnc = $mnc" >> $enb_conf
echo "mme_addr = 127.0.1.100" >> $enb_conf
echo "gtp_bind_addr = 127.0.1.1" >> $enb_conf
echo "s1c_bind_addr = 127.0.1.1" >> $enb_conf
echo "n_prb = 50" >> $enb_conf
echo "" >> $enb_conf
echo "[enb_files]" >> $enb_conf
echo "sib_config = $sib_conf" >> $enb_conf
echo "rr_config = $rr_conf" >> $enb_conf
echo "drb_config = $drb_conf" >> $enb_conf
echo "" >> $enb_conf
echo "[rf]" >> $enb_conf
echo "dl_earfcn = $dl_earfcn" >> $enb_conf
echo "tx_gain = 80" >> $enb_conf
echo "rx_gain = 40" >> $enb_conf
echo "" >> $enb_conf
echo "[pcap]" >> $enb_conf
echo "enable = true" >> $enb_conf
echo "filename = $enb_pcap" >> $enb_conf
echo "s1ap_enable = true" >> $enb_conf
echo "s1ap_filename = $s1ap_pcap" >> $enb_conf
echo "[log]" >> $enb_conf
echo "all_level = info" >> $enb_conf
echo "all_hex_limit = 32" >> $enb_conf
echo "filename = $enb_log" >> $enb_conf
echo "file_max_size = -1" >> $enb_conf
echo "" >> $enb_conf
echo "[gui]" >> $enb_conf
echo "enable = false" >> $enb_conf
echo "" >> $enb_conf
echo "[scheduler]" >> $enb_conf
echo "" >> $enb_conf
echo "[embms]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl.awgn]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl.fading]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl.delay]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl.rlf]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.dl.hst]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul.awgn]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul.fading]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul.delay]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul.rlf]" >> $enb_conf
echo "" >> $enb_conf
echo "[channel.ul.hst]" >> $enb_conf
echo "" >> $enb_conf
echo "[expert]" >> $enb_conf

# Check if USRP is connected.
device_info=$(uhd_find_devices | grep B210)
while [ ${#device_info} == 0 ]
do
	echo -e "\033[31mNo USRP B210 found......\033[0m"
	echo -e "\033[31mPlease check if the device is connected......\033[0m"
	echo -e "\033[31mWaiting for the device to connect......\033[0m"
	sleep 5
	device_info=$(uhd_find_devices | grep B210)
done
# Device connected successfully.
echo -e "\033[32m${device_info} connect success......\033[0m"

#### test env
# start epc
srsepc $epc_conf > $epc_run_log 2>&1 &
srsepc_pid=$(ps -aux  | grep -v 'grep' | grep srsepc | awk '{print $2}')
# echo $srsepc_pid
echo "Start epc, pid=$srsepc_pid, the log is in $epc_run_log"

# add iptables
sleep 3 #sleep 3.waitting for the srsepc start...
iptables -t nat -A POSTROUTING -s 172.16.0.1/24 -o $network -j MASQUERADE

# start enb
/home/skygo/workspace/hjc/ltesystem/srsenb $enb_conf > $enb_run_log 2>&1 &
srsenb_pid=$(ps -aux | grep -v 'grep' | grep srsenb | awk '{print $2}')
# echo $srsenb_pid
echo -e "\033[32mSetting frequency: DL=$DL Mhz, UL=$UL MHz......\033[0m"
echo "Start enb, pid=$srsenb_pid, the log is in $enb_run_log"