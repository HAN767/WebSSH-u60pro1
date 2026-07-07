# 自动给监控脚本加权限并悄悄在后台启动
chmod +x ./network_monitor.sh 2>/dev/null
nohup ./network_monitor.sh > /dev/null 2>&1 &

# 自动在屏幕上打印出当前的监控结果
sleep 1
cat /tmp/network_monitor.log 2>/dev/null
