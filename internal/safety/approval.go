package safety

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// UserDecision 人工审批做出的决策
type UserDecision string

const (
	DecisionAllowOnce   UserDecision = "allow_once"   // 仅允许当前这一步
	DecisionAllowTask   UserDecision = "allow_task"   // 本任务内同类操作不再询问
	DecisionAllowAlways UserDecision = "allow_always" // 始终允许（写入免审核白名单）
	DecisionDeny        UserDecision = "deny"         // 拒绝执行
	DecisionTakeOver    UserDecision = "take_over"    // 人工直接接管
)

// Allows 检查该决策是否放行当前步骤
func (d UserDecision) Allows() bool {
	return d == DecisionAllowOnce || d == DecisionAllowTask || d == DecisionAllowAlways
}

// ApprovalState 审批单生命周期状态
type ApprovalState int

const (
	StateOpen      ApprovalState = iota // 等待人工审核
	StateDecided                        // 人工已做决策，等待调用方认领
	StateUsed                           // 已被动作调用方取走执行，单次失效
	StateWithdrawn                      // 已主动撤回
	StateExpired                        // 已超时（默认 60 秒无人应答自动拒绝）
)

func (s ApprovalState) String() string {
	switch s {
	case StateOpen:
		return "open"
	case StateDecided:
		return "decided"
	case StateUsed:
		return "used"
	case StateWithdrawn:
		return "withdrawn"
	case StateExpired:
		return "expired"
	default:
		return fmt.Sprintf("state(%d)", int(s))
	}
}

// ApprovalBinding 审批单安全绑定上下文
type ApprovalBinding struct {
	Device   string `json:"device"`
	Tool     string `json:"tool"`
	ArgsHash string `json:"args_hash"`
}

// ComputeArgsHash 计算参数的简要安全指纹，防止跨参数篡改
func ComputeArgsHash(args string) string {
	h := sha256.Sum256([]byte(args))
	return hex.EncodeToString(h[:8])
}

// ApprovalRequest 创建审批单的请求信息
type ApprovalRequest struct {
	Binding    ApprovalBinding `json:"binding"`
	Rule       string          `json:"rule"`
	Title      string          `json:"title"`
	Detail     string          `json:"detail,omitempty"`
	AppName    string          `json:"app_name,omitempty"`
	AppPackage string          `json:"app_package,omitempty"`
	AppIcon    string          `json:"app_icon,omitempty"`
	TimeoutSec int             `json:"timeout_sec,omitempty"`
	Intent     string          `json:"intent,omitempty"`
	Screenshot []byte          `json:"-"`
}

// ApprovalInfo 审批单对外的完整视图
type ApprovalInfo struct {
	ID         string          `json:"id"`
	Binding    ApprovalBinding `json:"binding"`
	Rule       string          `json:"rule"`
	Title      string          `json:"title"`
	Detail     string          `json:"detail,omitempty"`
	AppName    string          `json:"app_name,omitempty"`
	AppPackage string          `json:"app_package,omitempty"`
	AppIcon    string          `json:"app_icon,omitempty"`
	Intent     string          `json:"intent,omitempty"`
	State      ApprovalState   `json:"state"`
	Decision   UserDecision    `json:"decision,omitempty"`
	Note       string          `json:"note,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	Deadline   time.Time       `json:"deadline"`
	DecidedAt  *time.Time      `json:"decided_at,omitempty"`
}

// 错误定义
var (
	ErrApprovalNotFound = errors.New("approval not found")
	ErrBindingMismatch  = errors.New("approval binding mismatch (device, tool or args modified)")
	ErrApprovalNotOpen  = errors.New("approval is not in open state")
	ErrApprovalUsed     = errors.New("approval already consumed")
)

// ApprovalEntry 审批单内部实体
type ApprovalEntry struct {
	Info     ApprovalInfo
	doneChan chan struct{}
	timer    *time.Timer
}

// ApprovalCenter 人工审批中心服务
type ApprovalCenter struct {
	mu             sync.Mutex
	items          map[string]*ApprovalEntry
	defaultTimeout time.Duration
	onChanged      func()
}

// NewApprovalCenter 创建审批中心实例
func NewApprovalCenter(timeout time.Duration) *ApprovalCenter {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &ApprovalCenter{
		items:          make(map[string]*ApprovalEntry),
		defaultTimeout: timeout,
	}
}

func generateApprovalID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "ap_" + hex.EncodeToString(b[:])
}

// Open 创建并挂起一个新的审批单
func (ac *ApprovalCenter) Open(req ApprovalRequest) ApprovalInfo {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	id := generateApprovalID()
	timeout := ac.defaultTimeout
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	now := time.Now()
	deadline := now.Add(timeout)

	info := ApprovalInfo{
		ID:         id,
		Binding:    req.Binding,
		Rule:       req.Rule,
		Title:      req.Title,
		Detail:     req.Detail,
		AppName:    req.AppName,
		AppPackage: req.AppPackage,
		AppIcon:    req.AppIcon,
		Intent:     req.Intent,
		State:      StateOpen,
		CreatedAt:  now,
		Deadline:   deadline,
	}

	entry := &ApprovalEntry{
		Info:     info,
		doneChan: make(chan struct{}),
	}

	// 启动超时自动拒绝定时器
	entry.timer = time.AfterFunc(timeout, func() {
		ac.expire(id)
	})

	ac.items[id] = entry

	if ac.onChanged != nil {
		go ac.onChanged()
	}

	return info
}

// expire 内部超时处理函数
func (ac *ApprovalCenter) expire(id string) {
	ac.mu.Lock()
	entry, ok := ac.items[id]
	if !ok || entry.Info.State != StateOpen {
		ac.mu.Unlock()
		return
	}

	now := time.Now()
	entry.Info.State = StateExpired
	entry.Info.Decision = DecisionDeny
	entry.Info.Note = "超过审批等待时限，系统已自动拒绝。"
	entry.Info.DecidedAt = &now
	close(entry.doneChan)
	ac.mu.Unlock()

	if ac.onChanged != nil {
		go ac.onChanged()
	}
}

// Decide 人工或 UI 提交决策
func (ac *ApprovalCenter) Decide(id string, decision UserDecision, note string) error {
	ac.mu.Lock()
	entry, ok := ac.items[id]
	if !ok {
		ac.mu.Unlock()
		return ErrApprovalNotFound
	}
	if entry.Info.State != StateOpen {
		ac.mu.Unlock()
		return ErrApprovalNotOpen
	}

	if entry.timer != nil {
		entry.timer.Stop()
	}

	now := time.Now()
	entry.Info.State = StateDecided
	entry.Info.Decision = decision
	entry.Info.Note = note
	entry.Info.DecidedAt = &now
	close(entry.doneChan)
	ac.mu.Unlock()

	if ac.onChanged != nil {
		go ac.onChanged()
	}

	return nil
}

// Consume 动作执行方获取审批结果并将其标记为已使用（单次失效）
func (ac *ApprovalCenter) Consume(id string, binding ApprovalBinding) (ApprovalInfo, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	entry, ok := ac.items[id]
	if !ok {
		return ApprovalInfo{}, ErrApprovalNotFound
	}

	// 校验绑定关系
	if entry.Info.Binding.Device != binding.Device ||
		entry.Info.Binding.Tool != binding.Tool ||
		entry.Info.Binding.ArgsHash != binding.ArgsHash {
		return ApprovalInfo{}, ErrBindingMismatch
	}

	if entry.Info.State == StateUsed {
		return ApprovalInfo{}, ErrApprovalUsed
	}

	if entry.Info.State == StateOpen {
		return ApprovalInfo{}, ErrApprovalNotOpen
	}

	info := entry.Info
	entry.Info.State = StateUsed
	return info, nil
}

// Wait 阻塞等待审批得出结论或超时
func (ac *ApprovalCenter) Wait(ctx context.Context, id string, maxWait time.Duration) (ApprovalInfo, error) {
	ac.mu.Lock()
	entry, ok := ac.items[id]
	if !ok {
		ac.mu.Unlock()
		return ApprovalInfo{}, ErrApprovalNotFound
	}
	done := entry.doneChan
	ac.mu.Unlock()

	var timerCh <-chan time.Time
	if maxWait > 0 {
		t := time.NewTimer(maxWait)
		defer t.Stop()
		timerCh = t.C
	}

	select {
	case <-done:
		ac.mu.Lock()
		defer ac.mu.Unlock()
		return entry.Info, nil
	case <-timerCh:
		ac.mu.Lock()
		defer ac.mu.Unlock()
		return entry.Info, nil // 超时返回当前状态（仍可能是 StateOpen）
	case <-ctx.Done():
		return ApprovalInfo{}, ctx.Err()
	}
}

// List 获取当前活跃审批列表（按创建时间倒序）
func (ac *ApprovalCenter) List(includeDecided bool) []ApprovalInfo {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	var list []ApprovalInfo
	for _, entry := range ac.items {
		if entry.Info.State == StateOpen || (includeDecided && entry.Info.State == StateDecided) {
			list = append(list, entry.Info)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list
}

// Withdraw 主动撤回某个审批单
func (ac *ApprovalCenter) Withdraw(id string, reason string) error {
	ac.mu.Lock()
	entry, ok := ac.items[id]
	if !ok {
		ac.mu.Unlock()
		return ErrApprovalNotFound
	}
	if entry.Info.State != StateOpen {
		ac.mu.Unlock()
		return ErrApprovalNotOpen
	}

	if entry.timer != nil {
		entry.timer.Stop()
	}

	now := time.Now()
	entry.Info.State = StateWithdrawn
	entry.Info.Note = reason
	entry.Info.DecidedAt = &now
	close(entry.doneChan)
	ac.mu.Unlock()

	if ac.onChanged != nil {
		go ac.onChanged()
	}

	return nil
}
