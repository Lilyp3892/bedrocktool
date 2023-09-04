package pages

import (
	"context"
	"log"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"github.com/bedrock-tool/bedrocktool/ui"
	"github.com/bedrock-tool/bedrocktool/ui/gui/icons"
	"github.com/bedrock-tool/bedrocktool/ui/messages"
	"github.com/bedrock-tool/bedrocktool/utils"
	"github.com/bedrock-tool/bedrocktool/utils/commands"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type Router struct {
	UI         ui.UI
	Ctx        context.Context
	Wg         sync.WaitGroup
	MSAuth     *guiAuth
	Invalidate func()

	Theme   *material.Theme
	pages   map[string]func(*Router) Page
	current Page

	ModalNavDrawer *component.ModalNavDrawer
	NavAnim        component.VisibilityAnimation
	AppBar         *component.AppBar
	ModalLayer     *component.ModalLayer
	NonModalDrawer bool
	BottomBar      bool

	UpdateButton    *widget.Clickable
	updateAvailable bool

	popups []Popup
}

func NewRouter(ctx context.Context, invalidate func(), th *material.Theme) *Router {
	modal := component.NewModal()

	nav := component.NewNav("Navigation Drawer", "This is an example.")
	modalNav := component.ModalNavFrom(&nav, modal)

	bar := component.NewAppBar(modal)
	//bar.NavigationIcon = icon.MenuIcon

	na := component.VisibilityAnimation{
		State:    component.Invisible,
		Duration: time.Millisecond * 250,
	}
	r := &Router{
		Ctx:            ctx,
		Invalidate:     invalidate,
		Theme:          th,
		pages:          make(map[string]func(*Router) Page),
		MSAuth:         &guiAuth{},
		ModalLayer:     modal,
		ModalNavDrawer: modalNav,
		AppBar:         bar,
		NavAnim:        na,

		UpdateButton: &widget.Clickable{},
	}
	r.MSAuth.router = r
	return r
}

func (r *Router) Register(p func(*Router) Page, id string) {
	r.pages[id] = p
}

func (r *Router) SwitchTo(tag string) {
	pf, ok := r.pages[tag]
	if !ok {
		logrus.Errorf("unknown page %s", tag)
		return
	}
	p := pf(r)

	navItem := p.NavItem()
	r.current = p
	r.AppBar.Title = navItem.Name
	actions := p.Actions()
	if r.updateAvailable {
		actions = append(actions, component.SimpleIconAction(r.UpdateButton, &icons.ActionUpdate, component.OverflowAction{}))
	}
	r.AppBar.SetActions(actions, p.Overflow())
	r.Invalidate()
}

func (r *Router) PushPopup(p Popup) {
	r.popups = append(r.popups, p)
	r.Invalidate()
}

func (r *Router) RemovePopup(id string) {
	r.popups = slices.DeleteFunc(r.popups, func(p Popup) bool {
		return p.ID() == id
	})
	r.Invalidate()
}

func (r *Router) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	if r.UpdateButton.Clicked() {
		r.SwitchTo("update")
	}

	for _, event := range r.AppBar.Events(gtx) {
		switch event := event.(type) {
		case component.AppBarNavigationClicked:
			if r.NonModalDrawer {
				r.NavAnim.ToggleVisibility(gtx.Now)
			} else {
				r.ModalNavDrawer.Appear(gtx.Now)
				r.NavAnim.Disappear(gtx.Now)
			}
		case component.AppBarContextMenuDismissed:
			log.Printf("Context menu dismissed: %v", event)
		case component.AppBarOverflowActionClicked:
			log.Printf("Overflow action selected: %v", event)
		}
	}
	if r.ModalNavDrawer.NavDestinationChanged() {
		r.SwitchTo(r.ModalNavDrawer.CurrentNavDestination().(string))
	}
	paint.Fill(gtx.Ops, th.Palette.Bg)

	var children []layout.StackChild
	children = append(children, layout.Expanded(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X /= 3
				return r.ModalNavDrawer.NavDrawer.Layout(gtx, th, &r.NavAnim)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return r.current.Layout(gtx, th)
			}),
		)
	}))

	for _, p := range r.popups {
		p := p
		children = append(children, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return p.Layout(gtx, th)
		}))
	}

	content := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx, children...)
	})
	bar := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return r.AppBar.Layout(gtx, th, "Menu", "Actions")
	})
	flex := layout.Flex{Axis: layout.Vertical}
	if r.BottomBar {
		flex.Layout(gtx, content, bar)
	} else {
		flex.Layout(gtx, bar, content)
	}
	r.ModalLayer.Layout(gtx, th)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (r *Router) Handler(data interface{}) messages.MessageResponse {
	switch data := data.(type) {
	case messages.UpdateAvailable:
		r.updateAvailable = true
		r.AppBar.SetActions(append(
			r.current.Actions(),
			component.SimpleIconAction(r.UpdateButton, &icons.ActionUpdate, component.OverflowAction{}),
		), r.current.Overflow())
		r.Invalidate()
	case messages.ConnectState:
		if data == messages.ConnectStateBegin {
			r.PushPopup(NewConnect(r))
		}
	}

	for _, p := range r.popups {
		p.Handler(data)
	}

	return r.current.Handler(data)
}

func (r *Router) Execute(cmd commands.Command) {
	r.Wg.Add(1)
	go func() {
		defer r.Wg.Done()

		defer func() {
			if err := recover(); err != nil {
				err := err.(error)
				utils.PrintPanic(err)
				r.PushPopup(NewErrorPopup(r, err, func() {
					r.RemovePopup("connect")
					r.SwitchTo("settings")
				}, true))
			}
		}()

		err := cmd.Execute(r.Ctx, r.UI)
		if err != nil {
			logrus.Error(err)
			r.PushPopup(NewErrorPopup(r, err, func() {
				r.RemovePopup("connect")
				r.SwitchTo("settings")
			}, false))
		}
	}()
}
