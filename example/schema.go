package schema

type Example struct {
	ExampleID      string `rel:"primary_key"`
	ProjectID      string `rel:"foreign_key"`
	PrivateKeyPath string
	SequenceNumber int64
	Status         ExampleStatus `type:"enum,string"`
	Data           SSHData       `type:"json_struct"`
	Type           SSHType       `type:"enum,string"`
	CreatedAt      Timestamp
	UpdatedAt      Timestamp
}

type Example2 struct {
	ExampleID      string `rel:"primary_key"`
	ProjectID      string `rel:"foreign_key"`
	PrivateKeyPath string
	SequenceNumber int64
	Status         SSHStatus   `type:"enum,string"` // Pending,Stopped,Started,Killed
	Data           ExampleData `type:"json_struct"` // SSHPublicKey,SSHConfigPath,SSHStatus,SSHBugBop
	Type           SSHEnum     `type:"enum,int"`
	CreatedAt      Timestamp
	UpdatedAt      Timestamp
}
