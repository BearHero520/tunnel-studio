module tunnelstudio.local/studio

go 1.25.0

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260828
	golang.org/x/net v0.44.0
	golang.org/x/sys v0.36.0
)

require (
	golang.org/x/crypto v0.42.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	golang.zx2c4.com/wireguard v0.0.0-20261006164505-2631ce99a06f
)

replace github.com/amnezia-vpn/amneziawg-go/v3 => ./vendor-source/amneziawg-go

replace golang.zx2c4.com/wireguard => ./vendor-source/wireguard-go
