/*
 * tsproto2proto.js
 *
 * Reconstructs .proto definitions from ts-proto generated TypeScript
 * (packages/mezon-sdk/src/api/api.ts and src/rtapi/realtime.ts).
 *
 * The ts-proto `decode()` switch is an exact, lossless source of truth:
 *   - `case N:` gives the field number
 *   - the `reader.<method>()` call gives the proto scalar/wire type
 *   - `Type.decode(reader, ...)` gives message-typed fields
 *   - `XxxValue.decode(...).value` gives google.protobuf wrapper fields
 *   - `... as any` marks enum fields (type resolved from the interface)
 *   - `.push(...)` marks repeated fields
 *   - `message.f[entry.key] = entry.value` marks map fields
 *
 * We combine the decode info with the `export interface` declaration to
 * resolve named (message/enum) field types, then emit proto3.
 *
 * Run: node tools/tsproto2proto.js
 */
const fs = require("fs");
const path = require("path");

const SDK = path.resolve(__dirname, "../../mezon-sdk/src");
const OUT = path.resolve(__dirname, "../proto");

const WRAPPERS = {
  DoubleValue: "double",
  FloatValue: "float",
  Int64Value: "int64",
  UInt64Value: "uint64",
  Int32Value: "int32",
  UInt32Value: "uint32",
  BoolValue: "bool",
  StringValue: "string",
  BytesValue: "bytes",
};

// ---- parsing -------------------------------------------------------------

function parseEnums(src) {
  const enums = [];
  const re = /export enum (\w+) \{([\s\S]*?)\n\}/g;
  let m;
  while ((m = re.exec(src))) {
    const name = m[1];
    const body = m[2];
    const values = [];
    const vre = /^\s*(\w+)\s*=\s*(-?\d+)\s*,/gm;
    let v;
    while ((v = vre.exec(body))) {
      if (v[1] === "UNRECOGNIZED") continue; // ts-proto artifact, not a real proto value
      values.push({ name: v[1], num: parseInt(v[2], 10) });
    }
    enums.push({ name, values });
  }
  return enums;
}

// Parse `export interface NAME { ... }` -> { fieldName: tsType }
function parseInterfaces(src) {
  const ifaces = {};
  const re = /export interface (\w+) \{([\s\S]*?)\n\}/g;
  let m;
  while ((m = re.exec(src))) {
    const name = m[1];
    const body = m[2];
    const fields = {};
    // strip comments
    const clean = body.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    // field may be split across lines: `name?:\n | Type\n | undefined;` or `name: Type;`
    const fre = /(\w+)\??:\s*([\s\S]*?);/g;
    let f;
    while ((f = fre.exec(clean))) {
      const fname = f[1];
      let t = f[2].replace(/\s+/g, " ").trim();
      // strip union with undefined, leading pipe
      t = t.replace(/^\|\s*/, "").replace(/\s*\|\s*undefined\s*$/,"").trim();
      fields[fname] = t;
    }
    ifaces[name] = fields;
  }
  return ifaces;
}

// Parse the decode() body of each `export const NAME = { ... encode... decode... }`.
function parseMessages(src) {
  const messages = [];
  // find `export const NAME = {` ... matching the decode function within.
  const re = /export const (\w+) = \{/g;
  let m;
  while ((m = re.exec(src))) {
    const name = m[1];
    const start = m.index;
    // grab decode function body
    const decIdx = src.indexOf("decode(", start);
    if (decIdx < 0) continue;
    // ensure this decode belongs to this const (before next `export const`)
    const nextConst = src.indexOf("export const ", start + 1);
    if (nextConst > 0 && decIdx > nextConst) continue;
    const swIdx = src.indexOf("switch (tag >>> 3) {", decIdx);
    if (swIdx < 0) {
      messages.push({ name, fields: [] });
      continue;
    }
    // body until the closing of the while loop (the `if ((tag & 7) === 4` guard)
    const endIdx = src.indexOf("reader.skipType(tag & 7);", swIdx);
    const body = src.slice(swIdx, endIdx > 0 ? endIdx : src.indexOf("return message;", swIdx));
    const fields = parseDecodeCases(body);
    messages.push({ name, fields });
  }
  return messages;
}

function parseDecodeCases(body) {
  const fields = [];
  // split into case blocks
  const caseRe = /case (\d+):/g;
  const indices = [];
  let c;
  while ((c = caseRe.exec(body))) indices.push({ num: parseInt(c[1], 10), at: c.index });
  for (let i = 0; i < indices.length; i++) {
    const num = indices[i].num;
    const seg = body.slice(indices[i].at, i + 1 < indices.length ? indices[i + 1].at : body.length);
    const f = parseCase(num, seg);
    if (f) fields.push(f);
  }
  return fields;
}

function parseCase(num, seg) {
  // map: const entryN = TYPE.decode(reader, ...); ... message.NAME[entryN.key] = entryN.value;
  let mm = seg.match(/const (entry\w*) = (\w+)\.decode\(reader[\s\S]*?message\.(\w+)\[\1\.key\]/);
  if (mm) {
    return { num, name: mm[3], kind: "map", entryType: mm[2] };
  }
  // repeated message: message.NAME.push(TYPE.decode(reader
  mm = seg.match(/message\.(\w+)\.push\((\w+)\.decode\(reader/);
  if (mm) return { num, name: mm[1], kind: "message", type: mm[2], repeated: true };
  // repeated enum: message.NAME.push(reader.int32() as any)
  mm = seg.match(/message\.(\w+)\.push\(reader\.\w+\(\) as any\)/);
  if (mm) return { num, name: mm[1], kind: "enum", repeated: true };
  // repeated scalar: message.NAME.push( [longTo..(] reader.METHOD()
  mm = seg.match(/message\.(\w+)\.push\((?:\w+\()?reader\.(\w+)\(/);
  if (mm) return { num, name: mm[1], kind: "scalar", reader: mm[2], repeated: true };
  // wrapper: message.NAME = XxxValue.decode(reader, ...).value
  mm = seg.match(/message\.(\w+) = (\w+Value)\.decode\(reader[\s\S]*?\)\.value/);
  if (mm && WRAPPERS[mm[2]]) return { num, name: mm[1], kind: "wrapper", wrapper: mm[2] };
  // singular message: message.NAME = TYPE.decode(reader
  mm = seg.match(/message\.(\w+) = (\w+)\.decode\(reader/);
  if (mm) return { num, name: mm[1], kind: "message", type: mm[2] };
  // singular enum: message.NAME = reader.int32() as any
  mm = seg.match(/message\.(\w+) = reader\.\w+\(\) as any/);
  if (mm) return { num, name: mm[1], kind: "enum" };
  // singular scalar (possibly wrapped in longToString/longToNumber)
  mm = seg.match(/message\.(\w+) = (?:\w+\()?reader\.(\w+)\(/);
  if (mm) return { num, name: mm[1], kind: "scalar", reader: mm[2] };
  return null;
}

// ---- emission ------------------------------------------------------------

const READER_TO_PROTO = {
  double: "double", float: "float",
  int32: "int32", int64: "int64",
  uint32: "uint32", uint64: "uint64",
  sint32: "sint32", sint64: "sint64",
  fixed32: "fixed32", fixed64: "fixed64",
  sfixed32: "sfixed32", sfixed64: "sfixed64",
  bool: "bool", string: "string", bytes: "bytes",
};

function build() {
  const apiSrc = fs.readFileSync(path.join(SDK, "api/api.ts"), "utf8");
  const rtSrc = fs.readFileSync(path.join(SDK, "rtapi/realtime.ts"), "utf8");

  const apiEnums = parseEnums(apiSrc);
  const rtEnums = parseEnums(rtSrc);
  const apiIfaces = parseInterfaces(apiSrc);
  const rtIfaces = parseInterfaces(rtSrc);
  const apiMsgs = parseMessages(apiSrc);
  const rtMsgs = parseMessages(rtSrc);

  // per-file type ownership (names can collide across packages)
  const apiTypes = new Set([...apiEnums.map((e) => e.name), ...apiMsgs.map((m) => m.name)]);
  const rtTypes = new Set([...rtEnums.map((e) => e.name), ...rtMsgs.map((m) => m.name)]);

  // realtime imports some message types from ../api/api, occasionally aliased
  // (e.g. `ChannelDescription as ChannelDescription1`). Build localName -> realName.
  const rtAlias = {}; // localName -> realName (all resolve to mezon.api)
  const impMatch = rtSrc.match(/import \{([\s\S]*?)\} from "\.\.\/api\/api";/);
  if (impMatch) {
    for (const part of impMatch[1].split(",")) {
      const p = part.trim();
      if (!p) continue;
      const as = p.match(/^(\w+)\s+as\s+(\w+)$/);
      if (as) rtAlias[as[2]] = as[1];
      else rtAlias[p] = p;
    }
  }

  const enumNames = new Set([...apiEnums, ...rtEnums].map((e) => e.name));

  // detect map-entry message types to skip + record their kv
  const mapEntry = {}; // entryTypeName -> {key, value}
  function indexMapEntries(msgs) {
    for (const m of msgs) {
      for (const f of m.fields) {
        if (f.kind === "map") {
          const entry = msgs.find((x) => x.name === f.entryType) ||
            apiMsgs.concat(rtMsgs).find((x) => x.name === f.entryType);
          if (entry) {
            const keyF = entry.fields.find((x) => x.num === 1);
            const valF = entry.fields.find((x) => x.num === 2);
            mapEntry[f.entryType] = { key: keyF, value: valF };
          }
        }
      }
    }
  }
  indexMapEntries(apiMsgs);
  indexMapEntries(rtMsgs);
  const skip = new Set(Object.keys(mapEntry));

  // resolve a named field type to a (possibly qualified) proto type, for the file of `pkg`.
  function qualify(typeName, pkg) {
    if (pkg === "mezon.realtime") {
      // alias from api import (handles ChannelDescription1 -> mezon.api.ChannelDescription)
      if (rtAlias[typeName]) {
        const real = rtAlias[typeName];
        if (apiTypes.has(real)) return ".mezon.api." + real;
        return real;
      }
      if (rtTypes.has(typeName)) return typeName; // realtime owns it
      if (apiTypes.has(typeName)) return ".mezon.api." + typeName;
      return typeName;
    }
    // mezon.api
    if (apiTypes.has(typeName)) return typeName; // api owns it
    if (rtTypes.has(typeName)) return ".mezon.realtime." + typeName;
    return typeName;
  }

  function fieldProtoType(f, msgName, ifaces, pkg) {
    if (f.kind === "scalar") return READER_TO_PROTO[f.reader] || "bytes";
    if (f.kind === "wrapper") return "google.protobuf." + f.wrapper;
    if (f.kind === "message") return qualify(f.type, pkg);
    if (f.kind === "enum") {
      // resolve enum type name from the interface declaration
      const iface = ifaces[msgName] || {};
      let t = iface[f.name] || "";
      t = t.replace(/\[\]$/, "").replace(/\s*\|\s*undefined/g, "").trim();
      if (enumNames.has(t)) return qualify(t, pkg);
      return "int32"; // fallback (wire-compatible)
    }
    return "bytes";
  }

  function emitMessage(m, ifaces, pkg) {
    const lines = [];
    lines.push(`message ${m.name} {`);
    for (const f of m.fields) {
      if (f.kind === "map") {
        const kv = mapEntry[f.entryType];
        const kt = kv && kv.key ? (READER_TO_PROTO[kv.key.reader] || "string") : "string";
        let vt;
        if (kv && kv.value) {
          if (kv.value.kind === "message") vt = qualify(kv.value.type, pkg);
          else if (kv.value.kind === "wrapper") vt = "google.protobuf." + kv.value.wrapper;
          else vt = READER_TO_PROTO[kv.value.reader] || "string";
        } else vt = "string";
        lines.push(`  map<${kt}, ${vt}> ${f.name} = ${f.num};`);
        continue;
      }
      const pt = fieldProtoType(f, m.name, ifaces, pkg);
      const rep = f.repeated ? "repeated " : "";
      lines.push(`  ${rep}${pt} ${f.name} = ${f.num};`);
    }
    lines.push(`}`);
    return lines.join("\n");
  }

  function emitEnum(e) {
    const lines = [`enum ${e.name} {`];
    // proto3 needs a zero value first; ts-proto enums already start at 0
    const seen = new Set();
    for (const v of e.values) {
      if (seen.has(v.num)) { lines.push(`  // alias dropped: ${v.name} = ${v.num}`); continue; }
      seen.add(v.num);
      lines.push(`  ${v.name} = ${v.num};`);
    }
    lines.push(`}`);
    return lines.join("\n");
  }

  function emitFile(pkg, goPkg, imports, enums, msgs, ifaces) {
    const out = [];
    out.push(`syntax = "proto3";`);
    out.push(``);
    out.push(`package ${pkg};`);
    out.push(``);
    for (const imp of imports) out.push(`import "${imp}";`);
    if (imports.length) out.push(``);
    out.push(`option go_package = "${goPkg}";`);
    out.push(``);
    for (const e of enums) out.push(emitEnum(e) + "\n");
    for (const m of msgs) {
      if (skip.has(m.name)) continue;
      out.push(emitMessage(m, ifaces, pkg) + "\n");
    }
    return out.join("\n");
  }

  const apiProto = emitFile(
    "mezon.api",
    "github.com/quangledang23/mezon-sdk-go/api;api",
    ["google/protobuf/wrappers.proto"],
    apiEnums,
    apiMsgs,
    apiIfaces
  );
  const rtProto = emitFile(
    "mezon.realtime",
    "github.com/quangledang23/mezon-sdk-go/rtapi;rtapi",
    ["google/protobuf/wrappers.proto", "api/api.proto"],
    rtEnums,
    rtMsgs,
    rtIfaces
  );

  fs.mkdirSync(path.join(OUT, "api"), { recursive: true });
  fs.mkdirSync(path.join(OUT, "rtapi"), { recursive: true });
  fs.writeFileSync(path.join(OUT, "api/api.proto"), apiProto);
  fs.writeFileSync(path.join(OUT, "rtapi/realtime.proto"), rtProto);

  console.log(`api.proto: ${apiMsgs.length - countSkip(apiMsgs)} messages, ${apiEnums.length} enums`);
  console.log(`realtime.proto: ${rtMsgs.length - countSkip(rtMsgs)} messages, ${rtEnums.length} enums`);
  function countSkip(msgs) { return msgs.filter((m) => skip.has(m.name)).length; }
}

build();
