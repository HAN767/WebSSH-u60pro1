#!/bin/sh
# 4G/5G、物联卡/正常卡同步智能监控脚本

LOG_FILE="/tmp/network_monitor.log"
echo "=== 随身WiFi [双卡种+4G/5G] 智能同步监控已启动 ===" > $LOG_FILE

while true
do
    # 1. 智能卡种识别
    CURRENT_APN=$(atcmd "AT+CGDCONT?" 2>/dev/null | grep -oE '"[^"]+"' | head -n 1 | tr -d '"')
    APN_LOWER=$(echo "$CURRENT_APN" | tr 'A-Z' 'a-z')
    if echo "$APN_LOWER" | grep -qE "iot|m2m|vnet|card|bmlink|static"; then
        CARD_TYPE="物联卡 (IoT Card)"
    elif [ -z "$CURRENT_APN" ]; then
        CARD_TYPE="未检测到SIM卡"
    else
        CARD_TYPE="正常号卡 (手机卡/大流量卡)"
    fi

    # 2. 4G/5G 网络制式同步识别
    NET_MODE=$(atcmd "AT+CNSMOD?" 2>/dev/null | grep -oE '[0-9]+' | sed -n '2p')
    if [ "$NET_MODE" = "8" ]; then
        CURRENT_NET="5G (SA/NSA独立组网)"
    elif [ "$NET_MODE" = "7" ]; then
        CURRENT_NET="4G (LTE网络)"
    else
        CURRENT_NET="已联网 (4G/5G自动)"
    fi

    # 3. 签约速率自动同步
    AMBR_DOWN=$(atcmd "AT+C5GREG=2;+C5GREG?" 2>/dev/null | grep -oE '[0-9]+' | sed -n '3p')
    if [ -z "$AMBR_DOWN" ] || [ "$AMBR_DOWN" -eq 0 ]; then
        AMBR_DOWN=$(cat /sys/class/net/rmnet_data0/speed 2>/dev/null)
    fi

    if [ -z "$AMBR_DOWN" ] || [ "$AMBR_DOWN" -le 0 ]; then
        RATE_STR="限速策略未下发"
    else
        if [ "$AMBR_DOWN" -ge 900000 ] || [ "$AMBR_DOWN" -eq 1000 ]; then
            RATE_STR="1000 Mbps [白金满血千兆]"
        elif [ "$AMBR_DOWN" -ge 450000 ] || [ "$AMBR_DOWN" -eq 500 ]; then
            RATE_STR="500 Mbps [黄金优享速率]"
        else
            RATE_STR="300 Mbps 或以下"
        fi
    fi

    # 4. 写入输出日志
    TIMESTAMP=$(date "+%Y-%m-%d %H:%M:%S")
    echo "[$TIMESTAMP] 卡种: $CARD_TYPE | 网络: $CURRENT_NET | 速率: $RATE_STR" >> $LOG_FILE

    # 保持日志行数
    if [ $(wc -l < "$LOG_FILE") -gt 100 ]; then sed -i '1,20d' $LOG_FILE; fi
    sleep 10
done
