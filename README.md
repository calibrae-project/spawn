# Parallelcoin Daemon and Wallet

This is built on the Golang Bitcoin daemon Gocoin written by Piotr Narewski.

The first milestone set for this project is to implement a version that operates on the Parallelcoin network and implements its parameters as it operates at present, as well as adding Segwit addresses to be activated at a prescribed block height after release.

This will include the addition of a jsonRPC and gRPC interface for transaction broadcast and database query, the extension of the inbuilt web interface to provide a GUI written in Angular 2+ with Service Workers as the primary wallet application in addition to the cold wallet program and the interactive CLI built inside the Gocoin codebase.

The new Gocoin codebase introduces many useful features, the most notable being a faster database cache engine and compression of the blockchain to reduce storage requirements (Gocoin is perfectly capable of coping with Bitcoin so it will perform even better with the smaller DUO chain).

### Then there will be significant changes made to improve the utility and performance of the token.

- Block time of 60 seconds, expanded maximum block size to 8Mb

- Expansion of token precision to 256 bits as 42:214 bit fixed point values, to facilitate better a changed reward schedule that will diminish the reward by a constant percentage (exponential decay or half-life) per time that targets the prescribed maximum supply but allows indefinite mining of the token so long as precision is expanded again later when necessary.

- Stochastic moving average block-by-block difficulty adjustment - in order to prevent potential hashpower attacks, based on a deterministic selection of past blocks of a variable size derived from the head block hash (to prevent resonance springing up in response to a rapid jump or decline in network hash power on one of the PoW mining networks).

- Addition of Ethash Proof of Work derived from the Geth codebase, to reduce the chance of either SHA256 or Scrypt hashpower attacks, once an Ethash block appears, at the right schedule, the difficulty adjustment will be forced on the other solution types via the new difficulty adjustment algorithm.

#### And in future, and beyond the simple token itself:

- A Masternodes-like system with a reward share to fund full nodes for maintaining available replicas of the database and entry points to the peer to peer network, including a DHT (built from Bittorrent) based rapid sync system to synchronise both the blockchain as well as shared files, essentially a staking reward for this service to the network, though with a fixed stake requirement, payments per block (with liveness requirement).

- SporeDB based BFT distributed database system for building distributed application systems that use Parallelcoin as the bulk underlying clearance layer. At first a Reputation/Prediction Market Forum/Media monetisation system, then extending above this a distributed concurrent versioning system like Github, which will at first be primarily used to host the code of the system itself, and facilitate the incentivisation of developers to work on the code of the network, and of course, very importantly, a distributed exchange system, as each Spore BFT protocol application will issue tokens according to a timeline and consensus rate, in order to allow the market regulation of the activity and valuation of each application ecosystem.

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

	go get github.com/ParallelCoinTeam/duod


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
