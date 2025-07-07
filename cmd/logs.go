package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
	"github.com/spf13/cobra"
)

var (
	logsServerAddr string
	logsFilter     string
	logsFollow     bool
	logsTimeStamps bool
	logsForever    bool
	logsTimeout    int64
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [rule-name]",
	Short: "Stream logs from running devloop server",
	Long: `Stream real-time logs from a specific rule or all rules on a running devloop server.

This command connects to a running devloop server and streams log output from
rule executions. You can filter logs by rule name or watch all rules together.
The command will continue streaming until interrupted with Ctrl-C.

Examples:
  devloop logs                        # Stream logs from all rules
  devloop logs backend                # Stream logs from backend rule only
  devloop logs --server :6000 tests   # Stream from server on port 6000
  devloop logs --filter "error"       # Filter logs containing "error"
  devloop logs --timestamps backend   # Include timestamps in output`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var ruleName string
		if len(args) > 0 {
			ruleName = args[0]
		}
		runStreamLogs(ruleName)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().StringVarP(&logsServerAddr, "server", "s", "localhost:5555", "Server address (host:port)")
	logsCmd.Flags().StringVar(&logsFilter, "filter", "", "Filter logs containing this text")
	logsCmd.Flags().BoolVar(&logsFollow, "follow", true, "Follow log output (default true)")
	logsCmd.Flags().BoolVar(&logsTimeStamps, "timestamps", false, "Include timestamps in output")
	logsCmd.Flags().BoolVarP(&logsForever, "forever", "f", false, "Stream logs forever (no timeout)")
	logsCmd.Flags().Int64VarP(&logsTimeout, "timeout", "t", 3, "Timeout in seconds when no new logs (negative = forever)")
}

func runStreamLogs(ruleName string) {
	client, err := NewClient(Config{
		Address: logsServerAddr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Calculate timeout value
	var timeoutValue int64
	if logsForever {
		timeoutValue = -1 // Forever
	} else {
		timeoutValue = logsTimeout
		// Use default of 3 if timeout is 0
		if timeoutValue == 0 {
			timeoutValue = 3
		}
	}

	// Start streaming logs
	stream, err := client.StreamLogs(ruleName, logsFilter, timeoutValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to start log stream: %v\n", err)
		os.Exit(1)
	}

	// Setup interrupt handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start receiving logs in a goroutine
	logChan := make(chan *pb.LogLine)
	errChan := make(chan error)

	go func() {
		defer close(logChan)
		defer close(errChan)

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errChan <- err
				return
			}

			// Send each log line
			for _, line := range resp.Lines {
				logChan <- line
			}
		}
	}()

	// Print header
	if ruleName != "" {
		fmt.Printf("Streaming logs for rule: %s\n", ruleName)
	} else {
		fmt.Printf("Streaming logs for all rules\n")
	}
	if logsFilter != "" {
		fmt.Printf("Filter: %s\n", logsFilter)
	}
	fmt.Printf("Press Ctrl-C to stop...\n\n")

	// Main event loop
	for {
		select {
		case <-sigChan:
			fmt.Printf("\n\nLog streaming stopped.\n")
			return
		case err := <-errChan:
			fmt.Fprintf(os.Stderr, "Error: Log stream failed: %v\n", err)
			os.Exit(1)
		case logLine, ok := <-logChan:
			if !ok {
				// Channel closed - stream ended
				fmt.Printf("\nLog streaming ended.\n")
				return
			}
			if logLine == nil {
				log.Println("Did we come here?  should we quit?")
				continue
			}
			printLogLine(logLine)
		}
	}
}

func printLogLine(logLine *pb.LogLine) {
	var output string

	if logsTimeStamps && logLine.Timestamp > 0 {
		timestamp := time.Unix(0, logLine.Timestamp*int64(time.Millisecond))
		output += fmt.Sprintf("[%s] ", timestamp.Format("15:04:05.000"))
	}

	if logLine.RuleName != "" {
		output += fmt.Sprintf("[%s] ", logLine.RuleName)
	}

	output += logLine.Line

	fmt.Println(output)
}
