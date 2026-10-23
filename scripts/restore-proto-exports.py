#!/usr/bin/env python3
"""Restore MODLOCK_API export markers in generated protobuf C++ files.

aptre generate (go-protoc-wasi) cannot pass --cpp_out=dllexport_decl, so the
generated headers lack the SDK DLL export markers the old native protoc added.
This restores them at exactly the sites the C++ generator emits, keeping the
committed headers' ABI contract stable. Run after aptre generate.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

# The SDK DLL exports every message generated under src/ except the host
# app's own, which modlock-host compiles into itself.
SOURCE_DIR = Path(__file__).resolve().parent.parent / "src"
HOST_APP_DIR = SOURCE_DIR / "host_app"

# class <Msg> final : public ::google::protobuf::Message (pb.h)
CLASS_RE = re.compile(r"^class (\w+) final : public ::google::protobuf::Message", re.M)
# extern <Msg>DefaultTypeInternal _<Msg>_default_instance_; (pb.h)
DEFAULT_RE = re.compile(r"^extern (\w+)DefaultTypeInternal _\1_default_instance_;", re.M)
# extern const DescriptorTable descriptor_table_<mangled>; (pb.h)
DESCRIPTOR_RE = re.compile(r"^extern const ::google::protobuf::internal::DescriptorTable (descriptor_table_\w+);", re.M)
# extern const ClassDataFull <Msg>_class_data_; (pb.h)
CLASSDATA_RE = re.compile(r"^extern const ::google::protobuf::internal::ClassDataFull (\w+_class_data_);", re.M)
# struct TableStruct_<mangled> (pb.h)
TABLE_RE = re.compile(r"^struct (TableStruct_\w+) \{", re.M)
# #define PROTOBUF_INTERNAL_EXPORT_<mangled> (pb.h, bare)
EXPORT_RE = re.compile(r"^(#define PROTOBUF_INTERNAL_EXPORT_\w+)$", re.M)
# PROTOBUF_ATTRIBUTE_NO_DESTROY PROTOBUF_CONSTINIT (pb.cc, line end)
CONSTINIT_RE = re.compile(r"^(PROTOBUF_ATTRIBUTE_NO_DESTROY PROTOBUF_CONSTINIT)$", re.M)


def restore(path: Path) -> bool:
    text = path.read_text()
    original = text
    text = CLASS_RE.sub(r"class MODLOCK_API \1 final : public ::google::protobuf::Message", text)
    text = DEFAULT_RE.sub(r"MODLOCK_API extern \1DefaultTypeInternal _\1_default_instance_;", text)
    text = DESCRIPTOR_RE.sub(r"MODLOCK_API extern const ::google::protobuf::internal::DescriptorTable \1;", text)
    text = CLASSDATA_RE.sub(r"MODLOCK_API extern const ::google::protobuf::internal::ClassDataFull \1;", text)
    text = TABLE_RE.sub(r"struct MODLOCK_API \1 {", text)
    text = EXPORT_RE.sub(r"\1 MODLOCK_API", text)
    text = CONSTINIT_RE.sub(r"\1 MODLOCK_API", text)
    if text != original:
        path.write_text(text)
        return True
    return False


def main() -> int:
    changed = []
    for path in sorted(SOURCE_DIR.rglob("*.pb.*")):
        if HOST_APP_DIR in path.parents or path.suffix not in (".h", ".cc"):
            continue
        if restore(path):
            changed.append(str(path.relative_to(SOURCE_DIR)))
    print("restored MODLOCK_API in:", ", ".join(changed) if changed else "(none)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
