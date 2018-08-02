// main.go

package main

func main() {
	a := App{}
	a.Initialize() //init db
	a.Run(":8080") // localhost:port
}
