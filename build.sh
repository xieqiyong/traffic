#!/bin/sh
MAKE_NAME=${PWD}
echo "开始构建：${MAKE_NAME}..."
echo "编译动态库..."
export PCAPV=1.9.1 &&
tar xvf ${MAKE_NAME}/deploy/libpcap-$PCAPV.tar.gz &&
cd libpcap-$PCAPV &&
./configure --with-pcap=linux &&
make &&
PACKAGE_NAME=${PWD}
echo "动态库编译成功，路径：${PACKAGE_NAME}..."
sleep 1
echo "开始执行打包"
cd ${MAKE_NAME}
export LD_LIBRARY_PATH="-L ${PACKAGE_NAME}"
export CGO_LDFLAGS="-L ${PACKAGE_NAME}"
export CGO_CPPFLAGS="-I ${PACKAGE_NAME}"
go build .
sleep 1
rm -rf libpcap-${PCAPV}*
echo "构建成功"


