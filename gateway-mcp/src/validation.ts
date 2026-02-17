import { z } from "zod";

const requestOptions = z.object({
  request_id: z.string().optional(),
  timeout_ms: z.number().int().positive().max(30000).optional()
}).optional();

const selectorClause = z.object({
  field: z.enum([
    "SELECTOR_FIELD_REF_ID",
    "SELECTOR_FIELD_TEXT",
    "SELECTOR_FIELD_CONTENT_DESC",
    "SELECTOR_FIELD_RESOURCE_ID",
    "SELECTOR_FIELD_CLASS_NAME",
    "SELECTOR_FIELD_PACKAGE_NAME",
    "SELECTOR_FIELD_ENABLED",
    "SELECTOR_FIELD_CLICKABLE",
    "SELECTOR_FIELD_VISIBLE"
  ]),
  operator: z.enum([
    "SELECTOR_OPERATOR_EQ",
    "SELECTOR_OPERATOR_CONTAINS",
    "SELECTOR_OPERATOR_PREFIX",
    "SELECTOR_OPERATOR_SUFFIX",
    "SELECTOR_OPERATOR_REGEX"
  ]),
  value: z.string()
});

export const selectorSchema = z.object({
  clauses: z.array(selectorClause).min(1),
  match_all: z.boolean().default(true),
  within_ref_id: z.string().optional(),
  limit: z.number().int().positive().max(500).optional()
});

export const listDevicesSchema = z.object({
  platform_filter: z.enum(["PLATFORM_UNSPECIFIED", "PLATFORM_ANDROID", "PLATFORM_IOS"]).optional(),
  ready_only: z.boolean().optional(),
  options: requestOptions
});

export const activeAppSchema = z.object({
  device_id: z.string().min(1),
  options: requestOptions
});

export const uiTreeSchema = z.object({
  device_id: z.string().min(1),
  force_refresh: z.boolean().optional(),
  node_limit: z.number().int().positive().max(1000).optional(),
  depth_limit: z.number().int().positive().max(100).optional(),
  cursor: z.string().optional(),
  options: requestOptions
});

export const findElementsSchema = z.object({
  device_id: z.string().min(1),
  selector: selectorSchema.optional(),
  snapshot_id: z.string().optional(),
  limit: z.number().int().positive().max(500).optional(),
  cursor: z.string().optional(),
  include_nodes: z.boolean().optional(),
  options: requestOptions
}).superRefine((value, ctx) => {
  if (!value.selector && !value.snapshot_id) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, message: "selector or snapshot_id must be provided" });
  }
});

export const tapSchema = z.object({
  device_id: z.string().min(1),
  ref_id: z.string().optional(),
  coordinates: z.object({ x: z.number().int(), y: z.number().int() }).optional(),
  selector: selectorSchema.optional(),
  snapshot_id: z.string().optional(),
  tap_count: z.number().int().positive().max(5).optional(),
  options: requestOptions
});

export const typeSchema = z.object({
  device_id: z.string().min(1),
  ref_id: z.string().optional(),
  coordinates: z.object({ x: z.number().int(), y: z.number().int() }).optional(),
  selector: selectorSchema.optional(),
  snapshot_id: z.string().optional(),
  text: z.string(),
  clear_before_type: z.boolean().optional(),
  options: requestOptions
});

export const swipeSchema = z.object({
  device_id: z.string().min(1),
  start: z.object({ x: z.number().int(), y: z.number().int() }).optional(),
  end: z.object({ x: z.number().int(), y: z.number().int() }).optional(),
  direction: z.enum(["DIRECTION_UNSPECIFIED", "DIRECTION_UP", "DIRECTION_DOWN", "DIRECTION_LEFT", "DIRECTION_RIGHT"]).optional(),
  distance_px: z.number().int().positive().optional(),
  duration_ms: z.number().int().positive().max(5000).optional(),
  options: requestOptions
});

export const screenshotStreamSchema = z.object({
  device_id: z.string().min(1),
  max_fps: z.number().int().positive().max(30).optional(),
  jpeg_quality: z.number().int().min(1).max(100).optional(),
  max_width: z.number().int().positive().optional(),
  max_height: z.number().int().positive().optional(),
  max_frames: z.number().int().positive().max(30).optional(),
  options: requestOptions
});
