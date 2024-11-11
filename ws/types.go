package ws

type Client struct {
	clientId string
	userId   string
	conn     Conn
}

func (c Client) ClientId() string { return c.clientId }
func (c Client) UserId() string   { return c.userId }
func (c Client) Conn() Conn       { return c.conn }

type Conn interface {
	Write(message []byte) error
}

// type Person struct {
// 	userId string
// }

// func (p Person) GetOnlineClients() []Client
// func (c Person) UserId() string
