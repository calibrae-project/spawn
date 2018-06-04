// Parallelcoin DNS seeder
package main
import (
	"os"
	"log"
	"fmt"
	"github.com/urfave/cli"
)
func main() {
	app := cli.NewApp()
	app.Name = "seeder"
	app.Usage = "DNS seeder for the parallelcoin network"
	app.Version = "0.1.0"
	app.Action = func(c *cli.Context) error {
	  fmt.Println("DNS seeder for the parallelcoin network version", app.Version)
	  return nil
	}
	err := app.Run(os.Args)
	if err != nil {
	  log.Fatal(err)
	}
}