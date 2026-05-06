Go backend implementation notes

- Peer discovery candidate: mDNS or UDP broadcast on LAN.
- Sync trigger: file watcher + content hash.
- Transfer: chunked file transfer over TCP/QUIC.
- Version safety: store previous file snapshots in ".p2p-history/" before overwrite.

