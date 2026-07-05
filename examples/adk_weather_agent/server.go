// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io"
	"log"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"github.com/google/ax/proto"
)

// harnessServer implements proto.HarnessServiceServer by driving an ADK runner.
type harnessServer struct {
	proto.UnimplementedHarnessServiceServer
	runner *runner.Runner
}

// Connect handles one execution turn: it receives the HarnessStart, runs the ADK
// agent, streams the model's text output back as HarnessOutputs, and terminates
// with a HarnessEnd frame.
func (s *harnessServer) Connect(stream proto.HarnessService_ConnectServer) error {
	ctx := stream.Context()

	var conversationID string
	var userText string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		conversationID = req.GetConversationId()
		if start := req.GetStart(); start != nil {
			userText = latestUserText(start.GetMessages())
			// AX sends the full turn in one HarnessStart; process now.
			break
		}
	}

	if conversationID == "" {
		conversationID = "default"
	}
	if strings.TrimSpace(userText) == "" {
		return sendEnd(stream, conversationID, proto.State_STATE_FAILED, 3, "no user input provided")
	}

	log.Printf("conversation=%s handling: %q", conversationID, userText)

	msg := genai.NewContentFromText(userText, genai.RoleUser)
	var finalText strings.Builder
	for ev, err := range s.runner.Run(ctx, conversationID, conversationID, msg, agent.RunConfig{}) {
		if err != nil {
			log.Printf("conversation=%s agent error: %v", conversationID, err)
			return sendEnd(stream, conversationID, proto.State_STATE_FAILED, 13, err.Error())
		}
		// Emit only final (non-partial) model text to avoid duplicating streamed
		// deltas; the weather agent produces a single consolidated answer.
		text := finalEventText(ev)
		if text == "" {
			continue
		}
		finalText.WriteString(text)
		if err := sendOutput(stream, conversationID, text); err != nil {
			return err
		}
	}

	log.Printf("conversation=%s final answer: %q", conversationID, finalText.String())
	return sendEnd(stream, conversationID, proto.State_STATE_COMPLETED, 0, "")
}

// latestUserText returns the text of the last user message in msgs.
func latestUserText(msgs []*proto.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if role := m.GetRole(); role != "" && role != "user" {
			continue
		}
		if t := m.GetContent().GetText(); t != nil {
			return t.GetText()
		}
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if t := msgs[i].GetContent().GetText(); t != nil {
			return t.GetText()
		}
	}
	return ""
}

// finalEventText extracts consolidated model text from a non-partial event.
func finalEventText(ev *session.Event) string {
	if ev == nil || ev.Partial {
		return ""
	}
	if ev.Content == nil {
		return ""
	}
	// Only surface model/assistant authored text (skip user/tool echoes).
	if ev.Content.Role != "" && ev.Content.Role != genai.RoleModel {
		return ""
	}
	var b strings.Builder
	for _, p := range ev.Content.Parts {
		if p != nil && p.Text != "" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func sendOutput(stream proto.HarnessService_ConnectServer, conversationID, text string) error {
	return stream.Send(&proto.HarnessResponse{
		ConversationId: conversationID,
		Type: &proto.HarnessResponse_Outputs{
			Outputs: &proto.HarnessOutputs{
				Messages: []*proto.Message{{
					Role: "model",
					Content: &proto.Content{
						Type: &proto.Content_Text{
							Text: &proto.TextContent{Text: text},
						},
					},
				}},
			},
		},
	})
}

func sendEnd(stream proto.HarnessService_ConnectServer, conversationID string, state proto.State, code int32, desc string) error {
	end := &proto.HarnessEnd{State: state}
	if state == proto.State_STATE_FAILED {
		end.Error = &proto.Error{Code: code, Description: desc}
	}
	return stream.Send(&proto.HarnessResponse{
		ConversationId: conversationID,
		Type:           &proto.HarnessResponse_End{End: end},
	})
}
