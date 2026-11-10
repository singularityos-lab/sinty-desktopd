package fprint

type File struct {
	Kind   string
	Member string
	SHA256 string
}

type Driver struct {
	ID      string
	Vendor  string
	Name    string
	Version string
	License string
	Source  string
	URL     string
	SHA256  string
	Size    int64
	Arch    string
	Files   []File
}

var Catalog = []Driver{
	{
		ID:      "10a5:9800",
		Vendor:  "Fingerprint Cards (FPC)",
		Name:    "FPC MOH fingerprint driver",
		Version: "27.23.44.2",
		License: "Proprietary, Fingerprint Cards AB",
		Source:  "Ubuntu OEM driver archive (libfprint-2-tod1-fpc)",
		URL:     "https://ppa.launchpadcontent.net/libfprint-tod1-group/ppa/ubuntu/pool/main/libf/libfprint-2-tod1-fpc/libfprint-2-tod1-fpc_27.23.44.2+2404-0ubuntu1_amd64.deb",
		SHA256:  "5eea3bdfbcc84b56f8cb92d2705a647a8830a0dd13a498723b8752476d5b12d7",
		Size:    1050274,
		Arch:    "amd64",
		Files: []File{
			{"tod", "./usr/lib/x86_64-linux-gnu/libfprint-2/tod-1/libfprint-2-tod1-fpc-9800-27.23.44.2.so", "7d06ae4f4936b91a12b7fda9e64368b4c25c91a65d4acb990a198faf86488300"},
			{"usr", "./usr/lib/x86_64-linux-gnu/libfprint-tod-fpcbep-9800-27.23.44.2.so", "7087b4115db8fcb5c36ea7e5d42e062958406d65d9ee82e197fb3d2c285242e2"},
			{"udev", "./lib/udev/rules.d/60-libfprint-2-tod1-fpc.rules", "2185177b8e737d89a6ad71820bef0e2c5f85ba8d183dd434022771147242cee6"},
		},
	},
}
