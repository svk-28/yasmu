package mainlibs

import (
    "os"
)

func GetFQDN() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	// cname, err := net.LookupCNAME(hostname)
	// if err != nil {
		// return "", err
	// }
	// return strings.TrimSuffix(cname, "."), nil
    
    return hostname, nil
}
