package output

// StdOutput used for debugging, prints all incoming requests
type StdOutput struct {
}

// NewStdOutput constructor for StdOutput
func NewStdOutput() (i *StdOutput) {
	i = new(StdOutput)
	return
}

func (i *StdOutput) Write(data []byte) (int, error) {
	//b := bufio.NewReader(bytes.NewReader(data));
	//req, err := http.ReadRequest(b)
	//if(err != nil){
	//
	//}
	//fmt.Println(req)
	//fmt.Println(req.Method)
	//fmt.Println(req.URL)
	return len(data), nil
}

func (i *StdOutput) String() string {
	return "Stdout Output"
}