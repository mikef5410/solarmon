
PROJDIR = github.com/mikef5410/solarmon
CMDS = testRainforest testInverter

all: ${CMDS}

testRainforest: cmd/testRainforest/testRainforest.go rainforestEagle200Local.go solarEdgeModbus.go
	go get
	go build ${PROJDIR}/cmd/testRainforest

testInverter: cmd/testInverter/testInverter.go solarEdgeModbus.go rainforestEagle200Local.go
	go get
	go build ${PROJDIR}/cmd/testInverter

clean:
	rm -f ${CMDS}
	find . -name "#*" -o -name "*~" -exec rm -f {} \;
	go clean

