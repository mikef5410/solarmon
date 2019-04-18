
PROJDIR = github.com/mikef5410/solarmon
CMDS = testRainforest testInverter solarmon

all: ${CMDS}

testRainforest: cmd/testRainforest/testRainforest.go rainforestEagle200Local.go solarEdgeModbus.go
	go get -u -v
	go build ${PROJDIR}/cmd/testRainforest

testInverter: cmd/testInverter/testInverter.go solarEdgeModbus.go rainforestEagle200Local.go
	go get -u -v
	go build ${PROJDIR}/cmd/testInverter

solarmon: cmd/solarmon/main.go solarEdgeModbus.go rainforestEagle200Local.go
	go get -u -v
	go build ${PROJDIR}/cmd/solarmon

clean:
	rm -f ${CMDS}
	find . -name "#*" -o -name "*~" -exec rm -f {} \;
	go clean

