package agent

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	pb "github.com/panyam/devloop/gen/go/devloop/v1"
)

// handleGetConfigRequest handles a GetConfig request from the gateway.
func (o *OrchestratorV2) handleGetConfigRequest(correlationID string, req *pb.GetConfigRequest) {
	log.Printf("[devloop] Received GetConfig request for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)

	configJSON, err := yaml.Marshal(o.Config)
	if err != nil {
		log.Printf("[devloop] Error marshaling config for project %q: %v", req.GetProjectId(), err)
		// Send an error response back to gateway
		respMsg := &pb.DevloopMessage{
			CorrelationId: correlationID,
			Content: &pb.DevloopMessage_GetConfigResponse{
				GetConfigResponse: &pb.GetConfigResponse{ConfigJson: []byte(fmt.Sprintf("Error: %v", err))},
			},
		}
		select {
		case o.gatewaySendChan <- respMsg:
		default:
			log.Printf("[devloop] Failed to send GetConfigResponse for %s: channel full", correlationID)
		}
		return
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_GetConfigResponse{
			GetConfigResponse: &pb.GetConfigResponse{ConfigJson: configJSON},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent GetConfigResponse for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)
	default:
		log.Printf("[devloop] Failed to send GetConfigResponse for %s: channel full", correlationID)
	}
}

// handleGetRuleStatusRequest handles a GetRuleStatus request from the gateway.
func (o *OrchestratorV2) handleGetRuleStatusRequest(correlationID string, req *pb.GetRuleStatusRequest) {
	log.Printf("[devloop] Received GetRuleStatus request for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)

	status, ok := o.GetRuleStatus(req.GetRuleName())
	var pbStatus *pb.RuleStatus
	if ok && status != nil {
		pbStatus = &pb.RuleStatus{
			RuleName:        req.GetRuleName(),
			IsRunning:       status.IsRunning,
			StartTime:       status.StartTime.Unix(),
			LastBuildStatus: status.LastBuildStatus,
			LastBuildTime:   status.LastBuildTime.Unix(),
		}
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_GetRuleStatusResponse{
			GetRuleStatusResponse: &pb.GetRuleStatusResponse{
				RuleStatus: pbStatus,
			},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent GetRuleStatusResponse for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)
	default:
		log.Printf("[devloop] Failed to send GetRuleStatusResponse for %s: channel full", correlationID)
	}
}

// handleTriggerRuleRequest handles a TriggerRule request from the gateway.
func (o *OrchestratorV2) handleTriggerRuleRequest(correlationID string, req *pb.TriggerRuleRequest) {
	log.Printf("[devloop] Received TriggerRule request for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)

	err := o.TriggerRule(req.GetRuleName())
	var respErr string
	if err != nil {
		respErr = err.Error()
		log.Printf("[devloop] Error triggering rule %q: %v", req.GetRuleName(), err)
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_TriggerRuleResponse{
			TriggerRuleResponse: &pb.TriggerRuleResponse{
				Success: err == nil,
				Message: respErr,
			},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent TriggerRuleResponse for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)
	default:
		log.Printf("[devloop] Failed to send TriggerRuleResponse for %s: channel full", correlationID)
	}
}

// handleListWatchedPathsRequest handles a ListWatchedPaths request from the gateway.
func (o *OrchestratorV2) handleListWatchedPathsRequest(correlationID string, req *pb.ListWatchedPathsRequest) {
	log.Printf("[devloop] Received ListWatchedPaths request for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)

	paths := o.GetWatchedPaths()

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_ListWatchedPathsResponse{
			ListWatchedPathsResponse: &pb.ListWatchedPathsResponse{
				Paths: paths,
			},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent ListWatchedPathsResponse for project %q (correlation ID: %s)", req.GetProjectId(), correlationID)
	default:
		log.Printf("[devloop] Failed to send ListWatchedPathsResponse for %s: channel full", correlationID)
	}
}

// handleReadFileContentRequest handles a ReadFileContent request from the gateway.
func (o *OrchestratorV2) handleReadFileContentRequest(correlationID string, req *pb.ReadFileContentRequest) {
	log.Printf("[devloop] Received ReadFileContent request for project %q, file %q (correlation ID: %s)", req.GetProjectId(), req.GetPath(), correlationID)

	content, err := o.ReadFileContent(req.GetPath())
	if err != nil {
		log.Printf("[devloop] Error reading file %q: %v", req.GetPath(), err)
	}

	respMsg := &pb.DevloopMessage{
		CorrelationId: correlationID,
		Content: &pb.DevloopMessage_ReadFileContentResponse{
			ReadFileContentResponse: &pb.ReadFileContentResponse{
				Content: content,
			},
		},
	}

	select {
	case o.gatewaySendChan <- respMsg:
		log.Printf("[devloop] Sent ReadFileContentResponse for project %q, file %q (correlation ID: %s)", req.GetProjectId(), req.GetPath(), correlationID)
	default:
		log.Printf("[devloop] Failed to send ReadFileContentResponse for %s: channel full", correlationID)
	}
}

// handleGetHistoricalLogsRequest handles a GetHistoricalLogs request from the gateway.
func (o *OrchestratorV2) handleGetHistoricalLogsRequest(correlationID string, req *pb.GetHistoricalLogsRequest) {
	log.Printf("[devloop] Received GetHistoricalLogs request for project %q, rule %q (correlation ID: %s)", req.GetProjectId(), req.GetRuleName(), correlationID)

	// Read logs from file and send them back
	logFilePath := filepath.Join("./logs", fmt.Sprintf("%s.log", req.GetRuleName()))
	file, err := os.Open(logFilePath)
	if err != nil {
		log.Printf("[devloop] Error opening log file %q: %v", logFilePath, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Send historical log line
		logMsg := &pb.DevloopMessage{
			CorrelationId: correlationID,
			Content: &pb.DevloopMessage_LogLine{
				LogLine: &pb.LogLine{
					ProjectId: o.projectID,
					RuleName:  req.GetRuleName(),
					Line:      line,
					Timestamp: time.Now().UnixMilli(),
				},
			},
		}

		select {
		case o.gatewaySendChan <- logMsg:
		default:
			log.Printf("[devloop] Failed to send historical log line for %s: channel full", correlationID)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[devloop] Error reading historical log file %q: %v", logFilePath, err)
	}
}
