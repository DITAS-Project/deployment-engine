package cloudsigma

type ResourceType struct {
	UUID         string            `json:"uuid,omitempty"`
	Owner        *ResourceType     `json:"owner,omitempty"`
	Resources    []ResourceType    `json:"resources,omitempty"`
	Distribution string            `json:"distribution,omitempty"`
	Version      string            `json:"version,omitempty"`
	Name         string            `json:"name,omitempty"`
	Status       string            `json:"status,omitempty"`
	Tags         []ResourceType    `json:"tags,omitempty"`
	CPU          int               `json:"cpu,omitempty"`
	Mem          int               `json:"mem,omitempty"`
	VNCPassword  string            `json:"vnc_password,omitempty"`
	Drives       []ServerDriveType `json:"drives,omitempty"`
	NICS         []ServerNICType   `json:"nics,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
	Runtime      RuntimeType       `json:"runtime,omitempty"`
	SMP          int               `json:"smp,omitempty"`
	Size         int               `json:"size,omitempty"`
}

/*type Drive struct {
	UUID         string   `json:"uuid,omitempty"`
	Distribution string   `json:"distribution,omitempty"`
	Version      string   `json:"version,omitempty"`
	Name         string   `json:"name,omitempty"`
	Status       string   `json:"status,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}*/

type RequestCountType struct {
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	TotalCount int `json:"total_count"`
}

type RequestResponseType struct {
	Meta    RequestCountType `json:"meta,omitempty"`
	Objects []ResourceType   `json:"objects"`
}

type ServerDriveType struct {
	BootOrder  int          `json:"boot_order"`
	DevChannel string       `json:"dev_channel"`
	Device     string       `json:"device"`
	Drive      ResourceType `json:"drive"`
}

type ServerIPV4ConfType struct {
	Conf string `json:"conf"`
	IP   string `json:"ip,omitempty"`
	UUID string `json:"uuid,omitempty"`
}

type RuntimeType struct {
	NICs []NICInfoType `json:"nics"`
}

type NICInfoType struct {
	IPV4Info ResourceType `json:"ip_v4"`
}

type ServerNICType struct {
	IPV4Conf ServerIPV4ConfType `json:"ip_v4_conf"`
	Model    string             `json:"model"`
	VLAN     string             `json:"vlan,omitempty"`
}

/*type Server struct {
	UUID        string            `json:"uuid,omitempty"`
	Name        string            `json:"name"`
	CPU         int               `json:"cpu"`
	Mem         int               `json:"mem"`
	VNCPassword string            `json:"vnc_password"`
	Drives      []ServerDrive     `json:"drives,omitempty"`
	NICS        []ServerNICType   `json:"nics,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

type ServerRequestResponse struct {
	Objects []Server `json:"objects"`
}*/

type ActionResultType struct {
	Action string `json:"action"`
	Result string `json:"result"`
	UUID   string `json:"uuid"`
}

/*type Tag struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type TagRequestResponse struct {
	Objects []Tag `json:"objects"`
}*/
