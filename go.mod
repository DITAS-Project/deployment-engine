module deployment-engine

go 1.13

replace k8s.io/api => k8s.io/api v0.0.0-20190819141258-3544db3b9e44

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190819141724-e14f31a72a77

require (
	github.com/DITAS-Project/blueprint-go v0.0.0-20191008152613-bf6bc6aa3c2d
	github.com/go-resty/resty/v2 v2.0.0
	github.com/go-test/deep v1.0.4
	github.com/golang/snappy v0.0.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/sethvargo/go-password v0.1.2
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cast v1.3.0
	github.com/spf13/viper v1.4.0
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c // indirect
	github.com/xdg/stringprep v1.0.0 // indirect
	go.mongodb.org/mongo-driver v1.1.2
	golang.org/x/crypto v0.0.0-20191002192127-34f69633bfdc
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/api v0.0.0-20191003035645-10e821c09743
	k8s.io/apimachinery v0.0.0-20191003115452-c31ffd88d5d2
	k8s.io/client-go v11.0.0+incompatible
)
