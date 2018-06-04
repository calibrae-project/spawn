#!/bin/bash
sudo docker run -it --volume="`pwd`/work:/work" docker-parallelcoin "parallelcoind -datadir=/work"