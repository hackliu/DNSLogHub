package model

type DnsInfo struct {
	Type       DnsType `json:"Type"`
	Subdomain  string  `json:"Subdomain"`
	Ipaddress  string  `json:"Ipaddress"`
	IpLocation string  `json:"IpLocation"`
	Request    string  `json:"Request"`
	Time       int64   `json:"Time"`
}
