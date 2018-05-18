# Calibrae Spawn Cryptocurrency Daemon and Wallet

This is built on the Golang Bitcoin daemon Gocoin written by Piotr Narewski.

- Block time of 60 seconds
- Initial block reward of 10,000,000 reduced per each subsequent block with an annual supply growth of 3.125%, currency denomination stored in 42:214 bit fixed point values allow the block rewards to be precise to 63 decimal places and will not underflow for about 30 years. This is a big incentive to get early adoption as 96.875% of tokens ever to be minted will be produced in the first year. The supply rate is intended to intentionally be very stable against all other markets which have fluctuating annual supply growth rates, and thus will be both deflationary and hopefully become a benchmark against which other markets can be better quantified. The maximum supply approaches around 15,000,000,000,000 and thus does not need a cap as the exponential decay formula naturally has an upper limit that cannot be reached even if precision is endlessly increased.
- Uses a CPU-targeted Proof of Work algorithm tentatively named Hummingbird, which is based on Cuckoo Cycle, searches the number field created by a hash chain based on an initial random nonce, looking for difficulty-controlled lengths of nodes that form an unbranched cycle. This algorithm requires at least 12Gb of main memory and leverages the large on-die caches of CPUs using a proprietary binary tree algorithm designed especially for this search. The PoW is designed so that no custom made processor can perform better than a current generation CPU.

# About Gocoin

**Gocoin** is a full **Bitcoin** solution written in Go language (golang).

The software architecture is focused on maximum performance of the node
and cold storage security of the wallet.

The **client** (p2p node) is an application independent from the **wallet**.
It keeps the entire UTXO set in RAM, providing the best block processing performance on the market.
With a decent machine and a fast connection (e.g. 4 vCPUs from Google Cloud or Amazon AWS),
the node should sync the entire bitcoin block chain in less than 4 hours (as of chain height ~512000).

The **wallet** is designed to be used offline.
It is deterministic and password seeded.
As long as you remember the password, you do not need any backups ever.

# Requirements

## Hardware

**client**:

* 64-bit architecture OS and Go compiler.
* File system supporting files larger than 4GB.
* At least 15GB of system memory (RAM).


**wallet**:

* Any platform that you can make your Go (cross)compiler to build for (Raspberry Pi works).
* For security reasons make sure to use encrypted swap file (if there is a swap file).
* If you decide to store your password in a file, have the disk encrypted (in case it gets stolen).


## Operating System
Having hardware requirements met, any target OS supported by your Go compiler will do.
Currently that can be at least one of the following:

* Windows
* Linux
* OS X
* Free BSD

## Build environment
In order to build Gocoin yourself, you will need the following tools installed in your system:

* **Go** (version 1.8 or higher) - http://golang.org/doc/install
* **Git** - http://git-scm.com/downloads

If the tools mentioned above are all properly installed, you should be able to execute `go` and `git`
from your OS's command prompt without a need to specify full path to the executables.

### Linux

When building for Linux make sure to have `gcc` installed or delete file `lib/utxo/membind_linux.go`


# Getting sources

Use `go get` to fetch and install the source code files.
Note that source files get installed within your GOPATH folder.

	go get github.com/calibrae-project/spawn


# Building

## Client node
Go to the `client/` folder and execute `go build` there.


## Wallet
Go to the `wallet/` folder and execute `go build` there.


## Tools
Go to the `tools/` folder and execute:

	go build btcversig.go

Repeat the `go build` for each source file of the tool you want to build.

# Binaries

Windows or Linux (amd64) binaries can be downloaded from

 * https://sourceforge.net/projects/gocoin/files/?source=directory

Please note that the binaries are usually not up to date.
I strongly encourage everyone to build the binaries himself.

# Development
Although it is an open source project, I am sorry to inform you that I will not merge in any pull requests.
The reason is that I want to stay an explicit author of this software, to keep a full control over its
licensing. If you are missing some functionality, just describe me your needs and I will see what I can do
for you. But if you want your specific code in, please fork and develop your own repo.

# Support
The official web page of the project is served at <a href="http://gocoin.pl">gocoin.pl</a>
where you can find extended documentation, including **User Manual**.

Please do not log github issues when you only have questions concerning this software.
Instead see [Contact](http://gocoin.pl/gocoin_links.html) page at [gocoin.pl](http://gocoin.pl) website
for possible ways of contacting me.
