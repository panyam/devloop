# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# NO CHECKED-IN PROTOBUF GENCODE
# source: devloop/v1/agents.proto
# Protobuf Python Version: 6.31.1
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import runtime_version as _runtime_version
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
_runtime_version.ValidateProtobufRuntimeVersion(
    _runtime_version.Domain.PUBLIC,
    6,
    31,
    1,
    '',
    'devloop/v1/agents.proto'
)
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()


from google.api import annotations_pb2 as google_dot_api_dot_annotations__pb2
from google.api import http_pb2 as google_dot_api_dot_http__pb2
from google.protobuf import empty_pb2 as google_dot_protobuf_dot_empty__pb2
from mcp.v1 import annotations_pb2 as mcp_dot_v1_dot_annotations__pb2
from devloop.v1 import models_pb2 as devloop_dot_v1_dot_models__pb2


DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x17\x64\x65vloop/v1/agents.proto\x12\ndevloop.v1\x1a\x1cgoogle/api/annotations.proto\x1a\x15google/api/http.proto\x1a\x1bgoogle/protobuf/empty.proto\x1a\x18mcp/v1/annotations.proto\x1a\x17\x64\x65vloop/v1/models.proto\"\x12\n\x10GetConfigRequest\"?\n\x11GetConfigResponse\x12*\n\x06\x63onfig\x18\x01 \x01(\x0b\x32\x12.devloop.v1.ConfigR\x06\x63onfig\"-\n\x0eGetRuleRequest\x12\x1b\n\trule_name\x18\x01 \x01(\tR\x08ruleName\"7\n\x0fGetRuleResponse\x12$\n\x04rule\x18\x01 \x01(\x0b\x32\x10.devloop.v1.RuleR\x04rule\"\x19\n\x17ListWatchedPathsRequest\"0\n\x18ListWatchedPathsResponse\x12\x14\n\x05paths\x18\x01 \x03(\tR\x05paths\"1\n\x12TriggerRuleRequest\x12\x1b\n\trule_name\x18\x01 \x01(\tR\x08ruleName\"I\n\x13TriggerRuleResponse\x12\x18\n\x07success\x18\x01 \x01(\x08R\x07success\x12\x18\n\x07message\x18\x02 \x01(\tR\x07message\"b\n\x11StreamLogsRequest\x12\x1b\n\trule_name\x18\x01 \x01(\tR\x08ruleName\x12\x16\n\x06\x66ilter\x18\x02 \x01(\tR\x06\x66ilter\x12\x18\n\x07timeout\x18\x03 \x01(\x03R\x07timeout\"?\n\x12StreamLogsResponse\x12)\n\x05lines\x18\x01 \x03(\x0b\x32\x13.devloop.v1.LogLineR\x05lines2\xa0\x04\n\x0c\x41gentService\x12Y\n\tGetConfig\x12\x1c.devloop.v1.GetConfigRequest\x1a\x1d.devloop.v1.GetConfigResponse\"\x0f\x82\xd3\xe4\x93\x02\t\x12\x07/config\x12_\n\x07GetRule\x12\x1a.devloop.v1.GetRuleRequest\x1a\x1b.devloop.v1.GetRuleResponse\"\x1b\x82\xd3\xe4\x93\x02\x15\x12\x13/status/{rule_name}\x12l\n\x0bTriggerRule\x12\x1e.devloop.v1.TriggerRuleRequest\x1a\x1f.devloop.v1.TriggerRuleResponse\"\x1c\x82\xd3\xe4\x93\x02\x16\"\x14/trigger/{rule_name}\x12u\n\x10ListWatchedPaths\x12#.devloop.v1.ListWatchedPathsRequest\x1a$.devloop.v1.ListWatchedPathsResponse\"\x16\x82\xd3\xe4\x93\x02\x10\x12\x0e/watched-paths\x12o\n\nStreamLogs\x12\x1d.devloop.v1.StreamLogsRequest\x1a\x1e.devloop.v1.StreamLogsResponse\" \x82\xd3\xe4\x93\x02\x1a\x12\x18/stream/logs/{rule_name}0\x01\x42\x93\x01\n\x0e\x63om.devloop.v1B\x0b\x41gentsProtoP\x01Z+github.com/panyam/devloop/gen/go/devloop/v1\xa2\x02\x03\x44XX\xaa\x02\nDevloop.V1\xca\x02\nDevloop\\V1\xe2\x02\x16\x44\x65vloop\\V1\\GPBMetadata\xea\x02\x0b\x44\x65vloop::V1b\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'devloop.v1.agents_pb2', _globals)
if not _descriptor._USE_C_DESCRIPTORS:
  _globals['DESCRIPTOR']._loaded_options = None
  _globals['DESCRIPTOR']._serialized_options = b'\n\016com.devloop.v1B\013AgentsProtoP\001Z+github.com/panyam/devloop/gen/go/devloop/v1\242\002\003DXX\252\002\nDevloop.V1\312\002\nDevloop\\V1\342\002\026Devloop\\V1\\GPBMetadata\352\002\013Devloop::V1'
  _globals['_AGENTSERVICE'].methods_by_name['GetConfig']._loaded_options = None
  _globals['_AGENTSERVICE'].methods_by_name['GetConfig']._serialized_options = b'\202\323\344\223\002\t\022\007/config'
  _globals['_AGENTSERVICE'].methods_by_name['GetRule']._loaded_options = None
  _globals['_AGENTSERVICE'].methods_by_name['GetRule']._serialized_options = b'\202\323\344\223\002\025\022\023/status/{rule_name}'
  _globals['_AGENTSERVICE'].methods_by_name['TriggerRule']._loaded_options = None
  _globals['_AGENTSERVICE'].methods_by_name['TriggerRule']._serialized_options = b'\202\323\344\223\002\026\"\024/trigger/{rule_name}'
  _globals['_AGENTSERVICE'].methods_by_name['ListWatchedPaths']._loaded_options = None
  _globals['_AGENTSERVICE'].methods_by_name['ListWatchedPaths']._serialized_options = b'\202\323\344\223\002\020\022\016/watched-paths'
  _globals['_AGENTSERVICE'].methods_by_name['StreamLogs']._loaded_options = None
  _globals['_AGENTSERVICE'].methods_by_name['StreamLogs']._serialized_options = b'\202\323\344\223\002\032\022\030/stream/logs/{rule_name}'
  _globals['_GETCONFIGREQUEST']._serialized_start=172
  _globals['_GETCONFIGREQUEST']._serialized_end=190
  _globals['_GETCONFIGRESPONSE']._serialized_start=192
  _globals['_GETCONFIGRESPONSE']._serialized_end=255
  _globals['_GETRULEREQUEST']._serialized_start=257
  _globals['_GETRULEREQUEST']._serialized_end=302
  _globals['_GETRULERESPONSE']._serialized_start=304
  _globals['_GETRULERESPONSE']._serialized_end=359
  _globals['_LISTWATCHEDPATHSREQUEST']._serialized_start=361
  _globals['_LISTWATCHEDPATHSREQUEST']._serialized_end=386
  _globals['_LISTWATCHEDPATHSRESPONSE']._serialized_start=388
  _globals['_LISTWATCHEDPATHSRESPONSE']._serialized_end=436
  _globals['_TRIGGERRULEREQUEST']._serialized_start=438
  _globals['_TRIGGERRULEREQUEST']._serialized_end=487
  _globals['_TRIGGERRULERESPONSE']._serialized_start=489
  _globals['_TRIGGERRULERESPONSE']._serialized_end=562
  _globals['_STREAMLOGSREQUEST']._serialized_start=564
  _globals['_STREAMLOGSREQUEST']._serialized_end=662
  _globals['_STREAMLOGSRESPONSE']._serialized_start=664
  _globals['_STREAMLOGSRESPONSE']._serialized_end=727
  _globals['_AGENTSERVICE']._serialized_start=730
  _globals['_AGENTSERVICE']._serialized_end=1274
# @@protoc_insertion_point(module_scope)
