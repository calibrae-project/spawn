FROM salttest/ubuntu-12.04

RUN apt-get update
RUN apt-get -y install build-essential git libboost-all-dev
RUN apt-get -y install libqtgui4
RUN apt-get -y install qt4-qmake libqt4-dev libssl-dev
RUN apt-get -y install wget curl libminiupnpc-dev libssl-dev
RUN wget 'http://download.oracle.com/berkeley-db/db-4.8.30.NC.tar.gz'
RUN tar -xzvf db-4.8.30.NC.tar.gz
RUN git clone https://github.com/marcetin/parallelcoin.git
RUN cd db-4.8.30.NC/build_unix/; \
    ../dist/configure --enable-cxx --prefix=/usr; \
    make -j`nproc`; \
    make install
RUN cd ~/parallelcoin/src; \
    make -f makefile.unix -j`nproc`
RUN cd ~/parallelcoin; \
    qmake; \
    make -j`nproc`
RUN cp /parallelcoin/parallelcoin-qt /parallelcoin/src/parallelcoind /usr/bin/

VOLUME [ "/work" ]
WORKDIR /work

CMD tail -f /dev/null