// main.go

package main

func main() {
	a := App{}
	a.Initialize("root", "root", "k8sql") //init db
	a.Run(":8080")                        // localhost:port
}
