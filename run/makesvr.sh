rm -f logsvr
cd ../core
go build -o ../run/logsvr .
cd ../run
chmod +x logsvr
echo "logsvr built success"