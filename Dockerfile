FROM salttest/ubuntu-12.04

ENV HOME /work

ENV USER_ID ${USER_ID:-1000}
ENV GROUP_ID ${GROUP_ID:-1000}

RUN groupadd -g ${GROUP_ID} parallelcoin \
	&& useradd -u ${USER_ID} -g parallelcoin -s /bin/bash -m -d /work parallelcoin

RUN apt-get update; \
    apt-get -y install \
        build-essential git libboost-all-dev libqtgui4 qt4-qmake libqt4-dev \
        libssl-dev wget curl libminiupnpc-dev libssl-dev

RUN wget 'http://download.oracle.com/berkeley-db/db-4.8.30.NC.tar.gz' \
    && tar -xzvf db-4.8.30.NC.tar.gz
RUN cd db-4.8.30.NC/build_unix/ \
    && ../dist/configure --enable-cxx --prefix=/usr \
    && make -j`nproc` \
    && make install

RUN git clone https://github.com/marcetin/parallelcoin.git

RUN cd parallelcoin/src \
    && make -f makefile.unix -j`nproc`

RUN cd parallelcoin \
    && qmake \
    && make -j`nproc`

RUN cp /parallelcoin/parallelcoin-qt /parallelcoin/src/parallelcoind /usr/bin/

VOLUME [ "/work" ]
WORKDIR work

EXPOSE 11047 11048

CMD tail -f /dev/null
