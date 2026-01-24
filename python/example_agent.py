#!/usr/bin/env python3
"""
Example Python agent using the GAR framework.

This demonstrates a simple agent that uppercases input text.
"""

from gar import Agent
import proto.gar_pb2 as pb2


def process(session_id, inputs):
    """Process incoming content list and yield responses"""
    for content in inputs:
        yield pb2.Content(
            role="assistant",
            type="text",
            mimetype="text/plain",
            data=f"Python processed (session {session_id}): {content.data.upper()}"
        )


def health_check():
    """Health check function that always returns healthy"""
    return True, "OK", {}


if __name__ == "__main__":
    agent = Agent(
        agent_id="python-agent",
        process_func=process,
        health_check_func=health_check
    )
    agent.serve(port=50051)
