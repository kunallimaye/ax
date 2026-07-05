# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Container image for the ADK-Go weather agent (a standalone HarnessService gRPC
# server). Build context: repository root, so the local `replace
# github.com/google/ax => ../..` in the agent's go.mod resolves.
#
#   docker build -f examples/adk_weather_agent/Dockerfile -t <tag> .

FROM golang:1.26 AS build
WORKDIR /src

# Copy the whole repo so the agent module's local replace of github.com/google/ax
# (../..) resolves during the build.
COPY . .

WORKDIR /src/examples/adk_weather_agent
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/adk-weather-agent .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/adk-weather-agent /adk-weather-agent
# Cloud Run injects PORT; the agent honors it (default 8080).
EXPOSE 8080
ENTRYPOINT ["/adk-weather-agent"]
