// Package induction preserves the public Induction API while delegating its
// implementation to internal packages. It exposes request/response models,
// pipeline types, runtime helpers, persistence helpers, and inference entry
// points for llama.cpp-compatible servers.
package induction

import (
	"context"
	"io"
	"net/http"
	"time"

	core "github.com/mwiater/induction/internal/core"
)

// This package preserves the public github.com/mwiater/induction API while
// the implementation lives in internal/core.

// ApplicationToolChain is the public API alias for the corresponding core type.
type ApplicationToolChain = core.ApplicationToolChain

// ApplicationToolHandler is the public API alias for the corresponding core type.
type ApplicationToolHandler = core.ApplicationToolHandler

// ArtifactPathContext is the public API alias for the corresponding core type.
type ArtifactPathContext = core.ArtifactPathContext

// Batch is the public API alias for the corresponding core type.
type Batch = core.Batch

// BatchConfig is the public API alias for the corresponding core type.
type BatchConfig = core.BatchConfig

// BatchExecutor is the public API alias for the corresponding core type.
type BatchExecutor = core.BatchExecutor

// BatchItem is the public API alias for the corresponding core type.
type BatchItem = core.BatchItem

// BatchItemConfig is the public API alias for the corresponding core type.
type BatchItemConfig = core.BatchItemConfig

// BatchItemStatus is the public API alias for the corresponding core type.
type BatchItemStatus = core.BatchItemStatus

// BatchStatus is the public API alias for the corresponding core type.
type BatchStatus = core.BatchStatus

// BatchSummary is the public API alias for the corresponding core type.
type BatchSummary = core.BatchSummary

// ChatCompletionControlRequest is the public API alias for the corresponding core type.
type ChatCompletionControlRequest = core.ChatCompletionControlRequest

// ChatCompletionControlResponse is the public API alias for the corresponding core type.
type ChatCompletionControlResponse = core.ChatCompletionControlResponse

// ChatRequest is the public API alias for the corresponding core type.
type ChatRequest = core.ChatRequest

// ChatSession is the public API alias for the corresponding core type.
type ChatSession = core.ChatSession

// ChatSessionSummary is the public API alias for the corresponding core type.
type ChatSessionSummary = core.ChatSessionSummary

// ChoiceLogprobs is the public API alias for the corresponding core type.
type ChoiceLogprobs = core.ChoiceLogprobs

// ClassificationConfig is the public API alias for the corresponding core type.
type ClassificationConfig = core.ClassificationConfig

// ClassificationResult is the public API alias for the corresponding core type.
type ClassificationResult = core.ClassificationResult

// Client is the public API alias for the corresponding core type.
type Client = core.Client

// ClientOption is the public API alias for the corresponding core type.
type ClientOption = core.ClientOption

// ClientOptions is the public API alias for the corresponding core type.
type ClientOptions = core.ClientOptions

// Config is the public API alias for the corresponding core type.
type Config = core.Config

// ContentPart is the public API alias for the corresponding core type.
type ContentPart = core.ContentPart

// DashboardConversation is the public API alias for the corresponding core type.
type DashboardConversation = core.DashboardConversation

// DashboardEvalAggregate is the public API alias for the corresponding core type.
type DashboardEvalAggregate = core.DashboardEvalAggregate

// DashboardEvalBenchmark is the public API alias for the corresponding core type.
type DashboardEvalBenchmark = core.DashboardEvalBenchmark

// DashboardEvalData is the public API alias for the corresponding core type.
type DashboardEvalData = core.DashboardEvalData

// DashboardEvalEngine is the public API alias for the corresponding core type.
type DashboardEvalEngine = core.DashboardEvalEngine

// DashboardEvalServer is the public API alias for the corresponding core type.
type DashboardEvalServer = core.DashboardEvalServer

// DashboardEvalSuite is the public API alias for the corresponding core type.
type DashboardEvalSuite = core.DashboardEvalSuite

// DashboardGenerateOptions is the public API alias for the corresponding core type.
type DashboardGenerateOptions = core.DashboardGenerateOptions

// DashboardLoadEvent is the public API alias for the corresponding core type.
type DashboardLoadEvent = core.DashboardLoadEvent

// DashboardMetrics is the public API alias for the corresponding core type.
type DashboardMetrics = core.DashboardMetrics

// DashboardModelData is the public API alias for the corresponding core type.
type DashboardModelData = core.DashboardModelData

// DashboardPerformance is the public API alias for the corresponding core type.
type DashboardPerformance = core.DashboardPerformance

// DashboardResponse is the public API alias for the corresponding core type.
type DashboardResponse = core.DashboardResponse

// DashboardRuntime is the public API alias for the corresponding core type.
type DashboardRuntime = core.DashboardRuntime

// DashboardRuntimeConfig is the public API alias for the corresponding core type.
type DashboardRuntimeConfig = core.DashboardRuntimeConfig

// DashboardSessionProvenance is the public API alias for the corresponding core type.
type DashboardSessionProvenance = core.DashboardSessionProvenance

// DashboardSnapshotObservation is the public API alias for the corresponding core type.
type DashboardSnapshotObservation = core.DashboardSnapshotObservation

// DashboardSource is the public API alias for the corresponding core type.
type DashboardSource = core.DashboardSource

// DashboardSpeculative is the public API alias for the corresponding core type.
type DashboardSpeculative = core.DashboardSpeculative

// DashboardTokens is the public API alias for the corresponding core type.
type DashboardTokens = core.DashboardTokens

// DashboardToolUsage is the public API alias for the corresponding core type.
type DashboardToolUsage = core.DashboardToolUsage

// DecisionCandidateResult is the public API alias for the corresponding core type.
type DecisionCandidateResult = core.DecisionCandidateResult

// DecisionConfig is the public API alias for the corresponding core type.
type DecisionConfig = core.DecisionConfig

// DecisionResult is the public API alias for the corresponding core type.
type DecisionResult = core.DecisionResult

// DownloadedResource is the public API alias for the corresponding core type.
type DownloadedResource = core.DownloadedResource

// Duration is the public API alias for the corresponding core type.
type Duration = core.Duration

// FileContentPart is the public API alias for the corresponding core type.
type FileContentPart = core.FileContentPart

// IconSet is the public API alias for the corresponding core type.
type IconSet = core.IconSet

// ImageData is the public API alias for the corresponding core type.
type ImageData = core.ImageData

// ImageURLPart is the public API alias for the corresponding core type.
type ImageURLPart = core.ImageURLPart

// InferenceChoice is the public API alias for the corresponding core type.
type InferenceChoice = core.InferenceChoice

// InferenceDataPurgeResult is the public API alias for the corresponding core type.
type InferenceDataPurgeResult = core.InferenceDataPurgeResult

// InferenceFunctionCall is the public API alias for the corresponding core type.
type InferenceFunctionCall = core.InferenceFunctionCall

// InferenceResponse is the public API alias for the corresponding core type.
type InferenceResponse = core.InferenceResponse

// InferenceResponseMessage is the public API alias for the corresponding core type.
type InferenceResponseMessage = core.InferenceResponseMessage

// InferenceStreamChoice is the public API alias for the corresponding core type.
type InferenceStreamChoice = core.InferenceStreamChoice

// InferenceStreamChunk is the public API alias for the corresponding core type.
type InferenceStreamChunk = core.InferenceStreamChunk

// InferenceStreamDelta is the public API alias for the corresponding core type.
type InferenceStreamDelta = core.InferenceStreamDelta

// InferenceStreamToolCall is the public API alias for the corresponding core type.
type InferenceStreamToolCall = core.InferenceStreamToolCall

// InferenceToolCall is the public API alias for the corresponding core type.
type InferenceToolCall = core.InferenceToolCall

// InferenceUsage is the public API alias for the corresponding core type.
type InferenceUsage = core.InferenceUsage

// InputSet is the public API alias for the corresponding core type.
type InputSet = core.InputSet

// Interaction is the public API alias for the corresponding core type.
type Interaction = core.Interaction

// LogConfig is the public API alias for the corresponding core type.
type LogConfig = core.LogConfig

// Logger is the public API alias for the corresponding core type.
type Logger = core.Logger

// MCPApprovalFunc is the public API alias for the corresponding core type.
type MCPApprovalFunc = core.MCPApprovalFunc

// MCPServerConfig is the public API alias for the corresponding core type.
type MCPServerConfig = core.MCPServerConfig

// MCPTool is the public API alias for the corresponding core type.
type MCPTool = core.MCPTool

// Message is the public API alias for the corresponding core type.
type Message = core.Message

// MetricsData is the public API alias for the corresponding core type.
type MetricsData = core.MetricsData

// ModelCapabilities is the public API alias for the corresponding core type.
type ModelCapabilities = core.ModelCapabilities

// ModelCatalogEntry is the public API alias for the corresponding core type.
type ModelCatalogEntry = core.ModelCatalogEntry

// ModelInspection is the public API alias for the corresponding core type.
type ModelInspection = core.ModelInspection

// ModelLifecycleEvent is the public API alias for the corresponding core type.
type ModelLifecycleEvent = core.ModelLifecycleEvent

// ModelLoadProgress is the public API alias for the corresponding core type.
type ModelLoadProgress = core.ModelLoadProgress

// ModelManagerConfig is the public API alias for the corresponding core type.
type ModelManagerConfig = core.ModelManagerConfig

// ModelRuntimeConfig is the public API alias for the corresponding core type.
type ModelRuntimeConfig = core.ModelRuntimeConfig

// ModelRuntimeError is the public API alias for the corresponding core type.
type ModelRuntimeError = core.ModelRuntimeError

// ModelRuntimeState is the public API alias for the corresponding core type.
type ModelRuntimeState = core.ModelRuntimeState

// ModelSnapshot is the public API alias for the corresponding core type.
type ModelSnapshot = core.ModelSnapshot

// Pipeline is the public API alias for the corresponding core type.
type Pipeline = core.Pipeline

// PipelineArtifact is the public API alias for the corresponding core type.
type PipelineArtifact = core.PipelineArtifact

// PipelineItemResult is the public API alias for the corresponding core type.
type PipelineItemResult = core.PipelineItemResult

// PipelineParameters is the public API alias for the corresponding core type.
type PipelineParameters = core.PipelineParameters

// PipelineQuality is the public API alias for the corresponding core type.
type PipelineQuality = core.PipelineQuality

// PipelineQualityGate is the public API alias for the corresponding core type.
type PipelineQualityGate = core.PipelineQualityGate

// PipelineRetry is the public API alias for the corresponding core type.
type PipelineRetry = core.PipelineRetry

// PipelineStep is the public API alias for the corresponding core type.
type PipelineStep = core.PipelineStep

// PipelineStepEvent is the public API alias for the corresponding core type.
type PipelineStepEvent = core.PipelineStepEvent

// PipelineStepObserver is the public API alias for the corresponding core type.
type PipelineStepObserver = core.PipelineStepObserver

// PipelineValidation is the public API alias for the corresponding core type.
type PipelineValidation = core.PipelineValidation

// PipelineValidationIssue is the public API alias for the corresponding core type.
type PipelineValidationIssue = core.PipelineValidationIssue

// PipelineValidationRule is the public API alias for the corresponding core type.
type PipelineValidationRule = core.PipelineValidationRule

// PropsData is the public API alias for the corresponding core type.
type PropsData = core.PropsData

// ReasoningConfig is the public API alias for the corresponding core type.
type ReasoningConfig = core.ReasoningConfig

// ReasoningCutoffConfig is the public API alias for the corresponding core type.
type ReasoningCutoffConfig = core.ReasoningCutoffConfig

// ReasoningCutoffReason is the public API alias for the corresponding core type.
type ReasoningCutoffReason = core.ReasoningCutoffReason

// ReasoningCutoffSnapshot is the public API alias for the corresponding core type.
type ReasoningCutoffSnapshot = core.ReasoningCutoffSnapshot

// ResourceBudgetConfig is the public API alias for the corresponding core type.
type ResourceBudgetConfig = core.ResourceBudgetConfig

// ResponseFormat is the public API alias for the corresponding core type.
type ResponseFormat = core.ResponseFormat

// RuntimeModel is the public API alias for the corresponding core type.
type RuntimeModel = core.RuntimeModel

// RuntimeOperation is the public API alias for the corresponding core type.
type RuntimeOperation = core.RuntimeOperation

// RuntimeStatus is the public API alias for the corresponding core type.
type RuntimeStatus = core.RuntimeStatus

// ServerInspection is the public API alias for the corresponding core type.
type ServerInspection = core.ServerInspection

// ServerRole is the public API alias for the corresponding core type.
type ServerRole = core.ServerRole

// SessionCleanupResult is the public API alias for the corresponding core type.
type SessionCleanupResult = core.SessionCleanupResult

// FailedSession describes a persisted session with a recorded failure.
type FailedSession = core.FailedSession

// SlotsData is the public API alias for the corresponding core type.
type SlotsData = core.SlotsData

// StepOutputConfig is the public API alias for the corresponding core type.
type StepOutputConfig = core.StepOutputConfig

// SwitchOption is the public API alias for the corresponding core type.
type SwitchOption = core.SwitchOption

// SwitchOptions is the public API alias for the corresponding core type.
type SwitchOptions = core.SwitchOptions

// SwitchResult is the public API alias for the corresponding core type.
type SwitchResult = core.SwitchResult

// TokenLogprob is the public API alias for the corresponding core type.
type TokenLogprob = core.TokenLogprob

// TokenLogprobPosition is the public API alias for the corresponding core type.
type TokenLogprobPosition = core.TokenLogprobPosition

// Tool is the public API alias for the corresponding core type.
type Tool = core.Tool

// ToolFunction is the public API alias for the corresponding core type.
type ToolFunction = core.ToolFunction

// UploadedFile is the public API alias for the corresponding core type.
type UploadedFile = core.UploadedFile

// WhenCondition is the public API alias for the corresponding core type.
type WhenCondition = core.WhenCondition

const (
	// DefaultAttachmentMaxBytes limits local attachment reads.
	DefaultAttachmentMaxBytes = core.DefaultAttachmentMaxBytes
	// RemoteImageMaxBytes limits remote image downloads.
	RemoteImageMaxBytes = core.RemoteImageMaxBytes
	// RemotePDFMaxBytes limits remote PDF downloads.
	RemotePDFMaxBytes = core.RemotePDFMaxBytes
	// RemoteFetchTimeout bounds remote attachment requests.
	RemoteFetchTimeout = core.RemoteFetchTimeout
	// DashboardSchemaVersion identifies the persisted dashboard schema.
	DashboardSchemaVersion = core.DashboardSchemaVersion
	// DefaultDashboardSessionsDirectory is the default session input directory for dashboard generation.
	DefaultDashboardSessionsDirectory = core.DefaultDashboardSessionsDirectory
	// DefaultDashboardEvalResultsDirectory is the default evaluation-results directory for dashboards.
	DefaultDashboardEvalResultsDirectory = core.DefaultDashboardEvalResultsDirectory
	// DefaultDashboardEvalConfigPath is the default evaluation-suite configuration path.
	DefaultDashboardEvalConfigPath = core.DefaultDashboardEvalConfigPath
	// DefaultDashboardIgnorePath is the default dashboard exclusion file.
	DefaultDashboardIgnorePath = core.DefaultDashboardIgnorePath
	// DefaultDashboardMetricsPath is the default generated metrics path.
	DefaultDashboardMetricsPath = core.DefaultDashboardMetricsPath
	// DefaultDashboardSIAMetricsPath is the default SIA metrics path.
	DefaultDashboardSIAMetricsPath = core.DefaultDashboardSIAMetricsPath
	// DefaultDashboardTemplatePath is the default dashboard template path.
	DefaultDashboardTemplatePath = core.DefaultDashboardTemplatePath
	// DefaultDashboardHTMLPath is the default generated dashboard path.
	DefaultDashboardHTMLPath = core.DefaultDashboardHTMLPath
	// BatchPending marks a batch that has not started validation.
	BatchPending = core.BatchPending
	// BatchValidating marks a batch whose inputs are being validated.
	BatchValidating = core.BatchValidating
	// BatchRunning marks a batch with active item execution.
	BatchRunning = core.BatchRunning
	// BatchCompleted marks a batch whose items all completed successfully.
	BatchCompleted = core.BatchCompleted
	// BatchCompletedWithErrors marks a batch with one or more failed items.
	BatchCompletedWithErrors = core.BatchCompletedWithErrors
	// BatchItemPending marks an item awaiting execution.
	BatchItemPending = core.BatchItemPending
	// BatchItemInvalid marks an item rejected during validation.
	BatchItemInvalid = core.BatchItemInvalid
	// BatchItemRunning marks an item currently executing.
	BatchItemRunning = core.BatchItemRunning
	// BatchItemCompleted marks an item that completed successfully.
	BatchItemCompleted = core.BatchItemCompleted
	// BatchItemFailed marks an item that failed during execution.
	BatchItemFailed = core.BatchItemFailed
	// ModelRuntimeUnknown indicates that runtime state is unavailable.
	ModelRuntimeUnknown = core.ModelRuntimeUnknown
	// ModelRuntimeUnloaded indicates that a model is not loaded.
	ModelRuntimeUnloaded = core.ModelRuntimeUnloaded
	// ModelRuntimeLoading indicates that a model is loading.
	ModelRuntimeLoading = core.ModelRuntimeLoading
	// ModelRuntimeLoaded indicates that a model is ready.
	ModelRuntimeLoaded = core.ModelRuntimeLoaded
	// ModelRuntimeSleeping indicates that a loaded model is idle.
	ModelRuntimeSleeping = core.ModelRuntimeSleeping
	// ServerRoleUnknown indicates that the server role is undetermined.
	ServerRoleUnknown = core.ServerRoleUnknown
	// ServerRoleRouter identifies a routing server.
	ServerRoleRouter = core.ServerRoleRouter
	// ServerRoleModel identifies a model-serving server.
	ServerRoleModel = core.ServerRoleModel
	// ReasoningCutoffReasonMaxTokens identifies a token-budget cutoff.
	ReasoningCutoffReasonMaxTokens = core.ReasoningCutoffReasonMaxTokens
	// ReasoningCutoffReasonMaxSeconds identifies a time-limit cutoff.
	ReasoningCutoffReasonMaxSeconds = core.ReasoningCutoffReasonMaxSeconds
	// ReasoningCutoffReasonMaxContext identifies a context-utilization cutoff.
	ReasoningCutoffReasonMaxContext = core.ReasoningCutoffReasonMaxContext
)

var (
	// ErrModelNotFound reports that a requested model is unavailable.
	ErrModelNotFound = core.ErrModelNotFound
	// ErrRuntimeUnsupported reports that the server lacks runtime model support.
	ErrRuntimeUnsupported = core.ErrRuntimeUnsupported
	// ErrModelLoadFailed reports a failed runtime model load.
	ErrModelLoadFailed = core.ErrModelLoadFailed
	// ErrModelUnloadFailed reports a failed runtime model unload.
	ErrModelUnloadFailed = core.ErrModelUnloadFailed
	// ErrRuntimeStateTimeout reports that runtime state did not settle in time.
	ErrRuntimeStateTimeout = core.ErrRuntimeStateTimeout
	// DefaultUnicode is the default terminal icon set.
	DefaultUnicode = core.DefaultUnicode
)

// BuildDashboardMetrics delegates to the corresponding core package operation.
func BuildDashboardMetrics(sessionsDirectory string) (*DashboardMetrics, error) {
	return core.BuildDashboardMetrics(sessionsDirectory)
}

// Chat delegates to the corresponding core package operation.
func Chat(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*Interaction, error) {
	return core.Chat(ctx, endpoint, req, options...)
}

// CheckHealth delegates to the corresponding core package operation.
func CheckHealth(endpoint string, options ...ClientOption) error {
	return core.CheckHealth(endpoint, options...)
}

// CleanSessions delegates to the corresponding core package operation.
func CleanSessions(directory string) (SessionCleanupResult, error) {
	return core.CleanSessions(directory)
}

// ListFailedSessions delegates to the corresponding core package operation.
func ListFailedSessions(directory string) ([]FailedSession, error) {
	return core.ListFailedSessions(directory)
}

// Cleanup delegates to the corresponding core package operation.
func Cleanup(out io.Writer) { core.Cleanup(out) }

// Complete delegates to the corresponding core package operation.
func Complete(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*Interaction, error) {
	return core.Complete(ctx, endpoint, req, options...)
}

// ExtractPDFText delegates to the corresponding core package operation.
func ExtractPDFText(path string, maxBytes int64) (string, error) {
	return core.ExtractPDFText(path, maxBytes)
}

// FetchRemoteSource delegates to the corresponding core package operation.
func FetchRemoteSource(source string, maxBytes int64) (*DownloadedResource, error) {
	return core.FetchRemoteSource(source, maxBytes)
}

// FileDataURL delegates to the corresponding core package operation.
func FileDataURL(path string, maxBytes int64) (dataURL, filename string, err error) {
	return core.FileDataURL(path, maxBytes)
}

// GenerateDashboardMetrics delegates to the corresponding core package operation.
func GenerateDashboardMetrics(options DashboardGenerateOptions) (*DashboardMetrics, error) {
	return core.GenerateDashboardMetrics(options)
}

// GenerateSnapshot delegates to the corresponding core package operation.
func GenerateSnapshot(ctx context.Context, endpoint string, req *ChatRequest, options ...ClientOption) (*ModelSnapshot, error) {
	return core.GenerateSnapshot(ctx, endpoint, req, options...)
}

// GetRuntimeStatus delegates to the corresponding core package operation.
func GetRuntimeStatus(ctx context.Context, endpoint string, options ...ClientOption) (*RuntimeStatus, error) {
	return core.GetRuntimeStatus(ctx, endpoint, options...)
}

// ImageDataURL delegates to the corresponding core package operation.
func ImageDataURL(path string, maxBytes int64) (string, error) {
	return core.ImageDataURL(path, maxBytes)
}

// Infer delegates to the corresponding core package operation.
func Infer(ctx context.Context, req *ChatRequest, options ...ClientOption) (*InferenceResponse, error) {
	return core.Infer(ctx, req, options...)
}

// InferApplicationToolsChat delegates to the corresponding core package operation.
func InferApplicationToolsChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, handler ApplicationToolHandler, options ...ClientOption) error {
	return core.InferApplicationToolsChat(ctx, req, in, out, handler, options...)
}

// InferChat delegates to the corresponding core package operation.
func InferChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return core.InferChat(ctx, req, in, out, options...)
}

// InferMCP delegates to the corresponding core package operation.
func InferMCP(ctx context.Context, req *ChatRequest, options ...ClientOption) (*InferenceResponse, error) {
	return core.InferMCP(ctx, req, options...)
}

// InferMCPChat delegates to the corresponding core package operation.
func InferMCPChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return core.InferMCPChat(ctx, req, in, out, options...)
}

// InferMCPChatWithApproval delegates to the corresponding core package operation.
func InferMCPChatWithApproval(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	return core.InferMCPChatWithApproval(ctx, req, in, out, approve, options...)
}

// InferMCPStream delegates to the corresponding core package operation.
func InferMCPStream(ctx context.Context, req *ChatRequest, out io.Writer, options ...ClientOption) error {
	return core.InferMCPStream(ctx, req, out, options...)
}

// InferMCPStreamChat delegates to the corresponding core package operation.
func InferMCPStreamChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return core.InferMCPStreamChat(ctx, req, in, out, options...)
}

// InferMCPStreamWithApproval delegates to the corresponding core package operation.
func InferMCPStreamWithApproval(ctx context.Context, req *ChatRequest, out io.Writer, approve MCPApprovalFunc, options ...ClientOption) error {
	return core.InferMCPStreamWithApproval(ctx, req, out, approve, options...)
}

// InferMCPWithApproval delegates to the corresponding core package operation.
func InferMCPWithApproval(ctx context.Context, req *ChatRequest, approve MCPApprovalFunc, options ...ClientOption) (*InferenceResponse, error) {
	return core.InferMCPWithApproval(ctx, req, approve, options...)
}

// InferStream delegates to the corresponding core package operation.
func InferStream(ctx context.Context, req *ChatRequest, out io.Writer, options ...ClientOption) error {
	return core.InferStream(ctx, req, out, options...)
}

// InferStreamChat delegates to the corresponding core package operation.
func InferStreamChat(ctx context.Context, req *ChatRequest, in io.Reader, out io.Writer, options ...ClientOption) error {
	return core.InferStreamChat(ctx, req, in, out, options...)
}

// InferStreamChunks delegates to the corresponding core package operation.
func InferStreamChunks(ctx context.Context, req *ChatRequest, yield func(InferenceStreamChunk) error, options ...ClientOption) error {
	return core.InferStreamChunks(ctx, req, yield, options...)
}

// IsRemoteSource delegates to the corresponding core package operation.
func IsRemoteSource(source string) bool { return core.IsRemoteSource(source) }

// ListLoadedModels delegates to the corresponding core package operation.
func ListLoadedModels(endpoint string, options ...ClientOption) error {
	return core.ListLoadedModels(endpoint, options...)
}

// ListModels delegates to the corresponding core package operation.
func ListModels(endpoint string, options ...ClientOption) error {
	return core.ListModels(endpoint, options...)
}

// LoadChatSession delegates to the corresponding core package operation.
func LoadChatSession(path string) (*ChatSession, error) { return core.LoadChatSession(path) }

// LoadConfig delegates to the corresponding core package operation.
func LoadConfig(path ...string) (*Config, error) { return core.LoadConfig(path...) }

// LoadModel delegates to the corresponding core package operation.
func LoadModel(ctx context.Context, endpoint, model string, options ...ClientOption) (*RuntimeOperation, error) {
	return core.LoadModel(ctx, endpoint, model, options...)
}

// LoadPipeline delegates to the corresponding core package operation.
func LoadPipeline(path string) (*Pipeline, error) { return core.LoadPipeline(path) }

// NewClient delegates to the corresponding core package operation.
func NewClient(ctx context.Context, endpoint string, options ...ClientOption) *Client {
	return core.NewClient(ctx, endpoint, options...)
}

// NewClientFromConfig delegates to the corresponding core package operation.
func NewClientFromConfig(ctx context.Context, options ...ClientOption) (*Client, error) {
	return core.NewClientFromConfig(ctx, options...)
}

// NewConfiguredLogger delegates to the corresponding core package operation.
func NewConfiguredLogger(config LogConfig) Logger { return core.NewConfiguredLogger(config) }

// PurgeInferenceData delegates to the corresponding core package operation.
func PurgeInferenceData() (InferenceDataPurgeResult, error) { return core.PurgeInferenceData() }

// RenderSessionTranscript delegates to the corresponding core package operation.
func RenderSessionTranscript(out io.Writer, session *ChatSession) error {
	return core.RenderSessionTranscript(out, session)
}

// ResolveArtifactPath delegates to the corresponding core package operation.
func ResolveArtifactPath(configuredPath string, ctx ArtifactPathContext) (string, error) {
	return core.ResolveArtifactPath(configuredPath, ctx)
}

// RunBatch delegates to the corresponding core package operation.
func RunBatch(ctx context.Context, pipeline *Pipeline, directory string, executor BatchExecutor, progress func(*Batch)) (*Batch, error) {
	return core.RunBatch(ctx, pipeline, directory, executor, progress)
}

// RunConsoleThemePreview delegates to the corresponding core package operation.
func RunConsoleThemePreview(ctx context.Context, in io.Reader, out io.Writer) error {
	return core.RunConsoleThemePreview(ctx, in, out)
}

// RunPipelineHeadless delegates to the corresponding core package operation.
func RunPipelineHeadless(ctx context.Context, pipeline *Pipeline, out io.Writer, options ...ClientOption) error {
	return core.RunPipelineHeadless(ctx, pipeline, out, options...)
}

// RunPipelineItem delegates to the corresponding core package operation.
func RunPipelineItem(ctx context.Context, client *Client, pipeline *Pipeline, input InputSet) (*PipelineItemResult, error) {
	return core.RunPipelineItem(ctx, client, pipeline, input)
}

// RunPipelineItemWithObserver delegates to the corresponding core package operation.
func RunPipelineItemWithObserver(ctx context.Context, client *Client, pipeline *Pipeline, input InputSet, observer PipelineStepObserver) (*PipelineItemResult, error) {
	return core.RunPipelineItemWithObserver(ctx, client, pipeline, input, observer)
}

// StreamChat delegates to the corresponding core package operation.
func StreamChat(ctx context.Context, endpoint string, req *ChatRequest, out io.Writer, options ...ClientOption) (*Interaction, error) {
	return core.StreamChat(ctx, endpoint, req, out, options...)
}

// StreamComplete delegates to the corresponding core package operation.
func StreamComplete(ctx context.Context, endpoint string, req *ChatRequest, out io.Writer, options ...ClientOption) (*Interaction, error) {
	return core.StreamComplete(ctx, endpoint, req, out, options...)
}

// UnloadModel delegates to the corresponding core package operation.
func UnloadModel(ctx context.Context, endpoint, model string, options ...ClientOption) (*RuntimeOperation, error) {
	return core.UnloadModel(ctx, endpoint, model, options...)
}

// WriteDashboardHTML delegates to the corresponding core package operation.
func WriteDashboardHTML(templatePath, path string, metrics *DashboardMetrics) error {
	return core.WriteDashboardHTML(templatePath, path, metrics)
}

// WriteDashboardMetrics delegates to the corresponding core package operation.
func WriteDashboardMetrics(path string, metrics *DashboardMetrics) error {
	return core.WriteDashboardMetrics(path, metrics)
}

// WithApplicationToolChain delegates to the corresponding core package operation.
func WithApplicationToolChain(chain ApplicationToolChain) ClientOption {
	return core.WithApplicationToolChain(chain)
}

// WithApplicationToolHandler delegates to the corresponding core package operation.
func WithApplicationToolHandler(handler ApplicationToolHandler) ClientOption {
	return core.WithApplicationToolHandler(handler)
}

// WithArtifactPathContext delegates to the corresponding core package operation.
func WithArtifactPathContext(value ArtifactPathContext) ClientOption {
	return core.WithArtifactPathContext(value)
}

// WithAutoExitAfterInitialChat delegates to the corresponding core package operation.
func WithAutoExitAfterInitialChat(enabled bool) ClientOption {
	return core.WithAutoExitAfterInitialChat(enabled)
}

// WithBatchMetadata delegates to the corresponding core package operation.
func WithBatchMetadata(batchID, itemID string) ClientOption {
	return core.WithBatchMetadata(batchID, itemID)
}

// WithConfigPath delegates to the corresponding core package operation.
func WithConfigPath(path string) ClientOption { return core.WithConfigPath(path) }

// WithHTTPClient delegates to the corresponding core package operation.
func WithHTTPClient(c *http.Client) ClientOption { return core.WithHTTPClient(c) }

// WithInferenceCompleted delegates to the corresponding core package operation.
func WithInferenceCompleted(callback func(*ModelSnapshot, string)) ClientOption {
	return core.WithInferenceCompleted(callback)
}

// WithInitialChatPrompt delegates to the corresponding core package operation.
func WithInitialChatPrompt(prompt string, autoSubmit bool) ClientOption {
	return core.WithInitialChatPrompt(prompt, autoSubmit)
}

// WithInitialModelSelection delegates to the corresponding core package operation.
func WithInitialModelSelection(enabled bool) ClientOption {
	return core.WithInitialModelSelection(enabled)
}

// WithLiveMetricsOverlay delegates to the corresponding core package operation.
func WithLiveMetricsOverlay(enabled bool) ClientOption { return core.WithLiveMetricsOverlay(enabled) }

// WithMCPFooter delegates to the corresponding core package operation.
func WithMCPFooter(enabled bool) ClientOption { return core.WithMCPFooter(enabled) }

// WithLoadWaitInterval delegates to the corresponding core package operation.
func WithLoadWaitInterval(d time.Duration) ClientOption { return core.WithLoadWaitInterval(d) }

// WithLogger delegates to the corresponding core package operation.
func WithLogger(logger Logger) ClientOption { return core.WithLogger(logger) }

// WithPipeline delegates to the corresponding core package operation.
func WithPipeline(pipeline *Pipeline) ClientOption { return core.WithPipeline(pipeline) }

// WithPollInterval delegates to the corresponding core package operation.
func WithPollInterval(d time.Duration) ClientOption { return core.WithPollInterval(d) }

// WithSessionSaved delegates to the corresponding core package operation.
func WithSessionSaved(callback func(string)) ClientOption { return core.WithSessionSaved(callback) }

// WithUnloadOthers delegates to the corresponding core package operation.
func WithUnloadOthers(enabled bool) SwitchOption { return core.WithUnloadOthers(enabled) }
