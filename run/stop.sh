pid=`ps x | grep logsvr | grep -v "grep" | awk '{print $1}'`

kill -9 $pid
echo "logsvr process killed"