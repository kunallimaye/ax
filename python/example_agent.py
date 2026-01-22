#!/usr/bin/env python3
"""
Example Python agent using the GAR framework.

This demonstrates a simple agent that uppercases input text.
"""

from gar import Agent
import proto.gar_pb2 as pb2


def process(content):
    """Process incoming content and return response"""
    return pb2.Content(
        role="assistant",
        type="text",
        mimetype="text/plain",
        data=f"Python processed: {content.data.upper()}"
    )


if __name__ == "__main__":
    agent = Agent(agent_id="python-agent", process_func=process)
    agent.serve(port=50051)
